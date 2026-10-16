using System.Buffers.Binary;
using System.Net.Sockets;
using HIDMaestro;

// SDK callbacks only store motor levels. One worker owns all runtime writes.
// Roster order is identical to the authenticated ready/input report order.
internal sealed class RumbleFeedback(NetworkStream stream, int count) : IDisposable
{
    private readonly int[] motors = new int[count];
    private readonly AutoResetEvent changed = new(false);
    private Thread? worker;
    private int stopped;

    public void Record(int index, HMOutputPacket packet)
    {
        if (Volatile.Read(ref stopped) != 0 || !TryDecode(packet, out ushort low, out ushort high)) return;
        Interlocked.Exchange(ref motors[index], unchecked((int)((uint)low | ((uint)high << 16))));
        try { changed.Set(); } catch (ObjectDisposedException) { }
    }

    private static bool TryDecode(HMOutputPacket packet, out ushort low, out ushort high)
    {
        low = high = 0;
        var bytes = packet.Data.Span;
        if (packet.Source == HMOutputSource.XInput)
        {
            if (bytes.Length == 4) // Raw XINPUT_VIBRATION, two little-endian WORDs.
            {
                low = BinaryPrimitives.ReadUInt16LittleEndian(bytes);
                high = BinaryPrimitives.ReadUInt16LittleEndian(bytes[2..]);
                return true;
            }
            if (bytes.Length == 5) // Pinned companion: command, size, low, high, reserved.
            {
                low = (ushort)(bytes[2] * 257);
                high = (ushort)(bytes[3] * 257);
                return true;
            }
        }
        // Xbox 360 USB rumble report. Never interpret LED/feature reports as motors.
        if (packet.Source == HMOutputSource.HidOutput && packet.ReportId == 0)
        {
            if (bytes.Length == 8 && bytes[0] == 0 && bytes[1] == 8 && bytes[2] == 0)
            {
                low = (ushort)(bytes[3] * 257); high = (ushort)(bytes[4] * 257);
                return true;
            }
            if (bytes.Length == 7 && bytes[0] == 8 && bytes[1] == 0)
            {
                low = (ushort)(bytes[2] * 257); high = (ushort)(bytes[3] * 257);
                return true;
            }
        }
        return false;
    }

    public void Start()
    {
        worker = new Thread(Run) { IsBackground = true, Name = "StageBoxRumbleFeedback" };
        worker.Start();
    }

    private void Run()
    {
        var bytes = new byte[count * 4];
        try
        {
            while (Volatile.Read(ref stopped) == 0)
            {
                changed.WaitOne(100); // Also heartbeat unchanged sustained effects.
                if (Volatile.Read(ref stopped) != 0) break;
                for (int i = 0; i < count; i++)
                    BinaryPrimitives.WriteInt32LittleEndian(bytes.AsSpan(i * 4), Volatile.Read(ref motors[i]));
                stream.Write(bytes);
            }
        }
        catch (Exception error) when (error is IOException or SocketException or ObjectDisposedException)
        {
            // Wake the main input reader so its normal finally owns device cleanup.
            stream.Close();
        }
    }

    public void Dispose()
    {
        if (Interlocked.Exchange(ref stopped, 1) != 0) return;
        changed.Set();
        if (worker != null && !worker.Join(750))
        {
            stream.Close();
            worker.Join();
        }
        changed.Dispose();
    }
}
