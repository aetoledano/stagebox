using System.Buffers.Binary;
using System.Diagnostics;
using System.Net;
using System.Net.Sockets;
using System.Runtime.InteropServices;
using System.Runtime.Versioning;
using System.Security.Cryptography;
using System.Security.Principal;
using System.Text;
using System.Text.Json;
using HIDMaestro;

[assembly: SupportedOSPlatform("windows10.0.17763")]

// One authenticated loopback connection and up to four controllers per invocation.
// The elevated helper accepts input reports only, never commands or file paths.
internal static class Program
{
    private static readonly (ushort Mask, HMButton Button)[] ButtonMap = {
        (0x1000, HMButton.A), (0x2000, HMButton.B), (0x4000, HMButton.X), (0x8000, HMButton.Y),
        (0x0100, HMButton.LeftBumper), (0x0200, HMButton.RightBumper),
        (0x0020, HMButton.Back), (0x0010, HMButton.Start),
        (0x0040, HMButton.LeftStick), (0x0080, HMButton.RightStick),
    };
    [StructLayout(LayoutKind.Sequential)]
    private struct XInputState { public uint Packet; public ushort Buttons; public byte LT, RT; public short LX, LY, RX, RY; }
    [DllImport("xinput1_4.dll")]
    private static extern uint XInputGetState(uint index, out XInputState state);

    private static bool[] Slots() => Enumerable.Range(0, 4).Select(i => XInputGetState((uint)i, out _) == 0).ToArray();
    private static void Send(NetworkStream stream, object value)
    {
        byte[] data = Encoding.UTF8.GetBytes(JsonSerializer.Serialize(value) + "\n");
        stream.Write(data);
    }

    public static int Main(string[] args)
    {
        if (args.Length == 1 && args[0] == "--recover-hidhide")
        {
            try { HidHideLease.RecoverStale(); return 0; }
            catch (Exception error) { Console.Error.WriteLine(error.Message); return 1; }
        }
        NetworkStream? stream = null;
        try
        {
            if (args.Length != 4 || !int.TryParse(args[0], out int port) || port < 1 || port > 65535 ||
                args[1].Length != 64 || !args[1].All(Uri.IsHexDigit) ||
                !int.TryParse(args[2], out int parentPID) || !long.TryParse(args[3], out long created)) return 2;
            using var parent = Process.GetProcessById(parentPID);
            if (parent.StartTime.ToUniversalTime().ToFileTimeUtc() != created || parent.HasExited) return 3;
            using var client = new TcpClient { NoDelay = true };
            using var connectTimeout = new CancellationTokenSource(TimeSpan.FromSeconds(10));
            client.ConnectAsync(IPAddress.Loopback, port, connectTimeout.Token).AsTask().GetAwaiter().GetResult();
            stream = client.GetStream();
            stream.ReadTimeout = 5000;
            stream.WriteTimeout = 5000;
            Send(stream, new { type = "hello", token = args[1] });
            var start = ReadStart(stream);
            if (!new WindowsPrincipal(WindowsIdentity.GetCurrent()).IsInRole(WindowsBuiltInRole.Administrator))
                throw new InvalidOperationException("Windows administrator permission is required for virtual input.");

            // Socket EOF on launcher exit is the primary watchdog. Also close it
            // when the exact parent process ends, including during SDK setup.
            using var watching = new CancellationTokenSource();
            _ = Task.Run(async () => {
                try { await parent.WaitForExitAsync(watching.Token); client.Dispose(); }
                catch (OperationCanceledException) { }
            });
            Console.SetOut(TextWriter.Null);
            Console.SetError(TextWriter.Null);
            HidHideSetup.EnsureInstalled();
            string executable = parent.MainModule?.FileName ?? throw new InvalidOperationException("Cannot identify the StageBox feeder executable.");
            using var hiding = HidHideLease.Begin(start, executable);
            if (parent.HasExited) return 0;
            using var context = new HMContext();
            context.LoadDefaultProfiles();
            // InstallDriver performs a system-wide ghost sweep. Never invoke it
            // on an existing install; don't remove another application's pads.
            if (!context.IsDriverInstalled) context.InstallDriver();
            var profile = context.GetProfile("xbox-360-wired") ?? throw new InvalidOperationException("Xbox 360 profile unavailable.");
            if (profile.Sticks.Count != 2 || profile.Triggers.Count != 2) throw new InvalidOperationException("Xbox profile axes are incompatible.");
            if (Slots().Count(value => !value) < start.Length) throw new InvalidOperationException("Not enough free Windows XInput slots for the virtual controllers.");
            var pads = new List<Pad>();
            var ready = new List<object>();
            try
            {
                for (int index = 0; index < start.Length; index++)
                {
                    bool[] before = Slots();
                    string key = "stagebox-native-" + args[1] + "-" + index;
                    string marker = "HM_" + BinaryPrimitives.ReadUInt64BigEndian(SHA256.HashData(Encoding.UTF8.GetBytes("HIDMaestro.DeviceIdentity.v1\n" + key))).ToString("X16");
                    var pad = new Pad(context.CreateController(profile, key), profile);
                    pads.Add(pad); // Own partial setup before any later failure.
                    int slot = -1;
                    var deadline = Stopwatch.StartNew();
                    while (deadline.Elapsed < TimeSpan.FromSeconds(5))
                    {
                        var added = Enumerable.Range(0, 4).Where(i => !before[i] && XInputGetState((uint)i, out _) == 0).ToArray();
                        if (added.Length > 1) throw new InvalidOperationException("Controller inventory changed during setup. Reconnect your controllers and retry.");
                        if (added.Length == 1) { slot = added[0]; break; }
                        if (parent.HasExited) return 0;
                        Thread.Sleep(20);
                    }
                    if (slot < 0) throw new InvalidOperationException("Windows did not expose a virtual Xbox controller through XInput.");
                    pad.Neutral();
                    ready.Add(new { marker, xinputIndex = slot, physicalHidden = true, physicalInstance = hiding.Instances[index] });
                }
                context.FinalizeNames();
                Send(stream, new { type = "ready", controllers = ready });
                stream.ReadTimeout = 1500;
                var bytes = new byte[12 * pads.Count];
                while (!parent.HasExited)
                {
                    stream.ReadExactly(bytes);
                    for (int index = 0; index < pads.Count; index++) pads[index].Submit(bytes, index * 12);
                }
            }
            finally
            {
                foreach (var pad in pads) { try { pad.Neutral(); } catch { } }
                context.Dispose(); // The SDK tears down its owned controllers in parallel.
                watching.Cancel();
            }
            return 0;
        }
        catch (EndOfStreamException) { return 0; }
        catch (IOException) { return 0; }
        catch (Exception error)
        {
            try { if (stream != null) Send(stream, new { type = "error", error = error.Message }); } catch { }
            return 1;
        }
    }

    private static string[] ReadStart(NetworkStream stream)
    {
        using var line = new MemoryStream();
        while (line.Length < 32768)
        {
            int value = stream.ReadByte();
            if (value < 0) throw new EndOfStreamException();
            if (value == '\n') {
                using var document = JsonDocument.Parse(line.ToArray());
                var root = document.RootElement;
                if (root.GetProperty("type").GetString() != "start") throw new InvalidOperationException("Invalid virtual-input setup request.");
                var paths = root.GetProperty("physicalPaths").EnumerateArray().Select(value => value.GetString() ?? "").ToArray();
                if (paths.Length is < 1 or > 4 || paths.Any(path => string.IsNullOrEmpty(path) || path.Length > 4096))
                    throw new InvalidOperationException("Invalid physical controller paths.");
                return paths;
            }
            line.WriteByte((byte)value);
        }
        throw new InvalidOperationException("Virtual-input setup request is too large.");
    }

    private sealed class Pad(HMController controller, HMProfile profile)
    {
        private HMGamepadState state = new() { Axes = HMGamepadStateHelpers.StandardAxes(profile) };
        private readonly HMAxis lx = profile.Sticks[0].XAxis, ly = profile.Sticks[0].YAxis,
            rx = profile.Sticks[1].XAxis, ry = profile.Sticks[1].YAxis,
            lt = profile.Triggers[0].Axis, rt = profile.Triggers[1].Axis;
        public void Neutral() => controller.SubmitState(new HMGamepadState { Axes = HMGamepadStateHelpers.StandardAxes(profile) });
        public void Submit(byte[] bytes, int offset)
        {
            ushort buttons = BinaryPrimitives.ReadUInt16LittleEndian(bytes.AsSpan(offset));
            HMButton mapped = HMButton.None;
            foreach (var binding in ButtonMap) if ((buttons & binding.Mask) != 0) mapped |= binding.Button;
            state.Buttons = mapped;
            bool up = (buttons & 1) != 0, down = (buttons & 2) != 0, left = (buttons & 4) != 0, right = (buttons & 8) != 0;
            int x = (right ? 1 : 0) - (left ? 1 : 0), y = (down ? 1 : 0) - (up ? 1 : 0);
            state.Hat = (x, y) switch {
                (0,-1) => HMHat.North, (1,-1) => HMHat.NorthEast, (1,0) => HMHat.East, (1,1) => HMHat.SouthEast,
                (0,1) => HMHat.South, (-1,1) => HMHat.SouthWest, (-1,0) => HMHat.West, (-1,-1) => HMHat.NorthWest,
                _ => HMHat.None,
            };
            var axes = state.Axes!;
            axes[lx] = Axis(bytes,offset+4,false); axes[ly] = Axis(bytes,offset+6,true);
            axes[rx] = Axis(bytes,offset+8,false); axes[ry] = Axis(bytes,offset+10,true);
            axes[lt] = bytes[offset+2]/255f; axes[rt] = bytes[offset+3]/255f;
            controller.SubmitState(state);
        }
    }

    private static float Axis(byte[] data, int offset, bool invert)
    {
        float value = (BinaryPrimitives.ReadInt16LittleEndian(data.AsSpan(offset)) + 32768f)/65535f;
        return invert ? 1f-value : value;
    }
}
