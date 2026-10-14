using System.Diagnostics;
using System.Runtime.InteropServices;
using System.Text;
using System.Text.Json;
using Microsoft.Win32;
using Microsoft.Win32.SafeHandles;

// HidHide 1.5.230's public control-device protocol. Keep control handles short:
// the driver grants only one configuration client access at a time.
internal sealed class HidHideControl : IDisposable
{
    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    internal static extern SafeFileHandle CreateFileW(string path, uint access, uint share, IntPtr security, uint creation, uint flags, IntPtr template);
    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern bool DeviceIoControl(SafeFileHandle handle, uint code, byte[]? input, uint inputSize, byte[]? output, uint outputSize, out uint returned, IntPtr overlapped);
    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    private static extern uint QueryDosDeviceW(string name, StringBuilder target, int count);
    [StructLayout(LayoutKind.Sequential)]
    private struct PropertyKey { public Guid Format; public uint ID; }
    [DllImport("cfgmgr32.dll", CharSet = CharSet.Unicode)]
    private static extern uint CM_Get_Device_Interface_PropertyW(string device, ref PropertyKey key, out uint type, byte[] data, ref uint size, uint flags);

    private readonly SafeFileHandle handle;
    private HidHideControl(SafeFileHandle value) { handle = value; }
    public static HidHideControl Open()
    {
        for (int attempt = 0; ; attempt++)
        {
            var handle = CreateFileW(@"\\.\HidHide", 0x80000000, 3, IntPtr.Zero, 3, 0x80, IntPtr.Zero);
            if (!handle.IsInvalid) return new(handle);
            int error = Marshal.GetLastWin32Error();
            handle.Dispose();
            if (error == 32 && attempt < 20) { Thread.Sleep(100); continue; }
            throw new HidHideAccessException(error);
        }
    }
    // CTL_CODE(32769, function, METHOD_BUFFERED, FILE_READ_DATA).
    private static uint Code(uint function) => (32769u << 16) | (1u << 14) | (function << 2);
    private uint Call(uint function, byte[]? input, byte[]? output)
    {
        if (!DeviceIoControl(handle, Code(function), input, (uint)(input?.Length ?? 0), output, (uint)(output?.Length ?? 0), out uint returned, IntPtr.Zero))
            throw new InvalidOperationException($"HidHide configuration failed (Windows error {Marshal.GetLastWin32Error()}). Close its configuration client and retry.");
        return returned;
    }
    public string[] List(uint function)
    {
        uint size = Call(function, null, null);
        if (size > 1024 * 1024 || size % 2 != 0) throw new InvalidOperationException("Invalid HidHide configuration size.");
        if (size == 0) return [];
        var data = new byte[size];
        uint returned = Call(function, null, data);
        if (returned > size || returned % 2 != 0) throw new InvalidOperationException("Invalid HidHide configuration response.");
        return Encoding.Unicode.GetString(data, 0, (int)returned).Split('\0', StringSplitOptions.RemoveEmptyEntries);
    }
    public void SetList(uint function, IEnumerable<string> values)
    {
        var data = Encoding.Unicode.GetBytes(string.Join('\0', values) + "\0\0");
        Call(function, data, null);
    }
    public bool Flag(uint function) { var data = new byte[1]; Call(function, null, data); return data[0] != 0; }
    public void SetFlag(uint function, bool value) => Call(function, [value ? (byte)1 : (byte)0], null);
    public void Dispose() => handle.Dispose();

    public static string InstanceID(string path)
    {
        if (path.Length > 4096 || !path.StartsWith(@"\\?\HID#", StringComparison.OrdinalIgnoreCase))
            throw new InvalidOperationException("The selected SDL controller has no supported Windows HID path.");
        var key = new PropertyKey { Format = new Guid("78c34fc8-104a-4aca-9ea4-524d52996e57"), ID = 256 };
        var data = new byte[4096]; uint size = (uint)data.Length;
        uint result = CM_Get_Device_Interface_PropertyW(path, ref key, out uint type, data, ref size, 0);
        if (result != 0 || type != 18 || size > data.Length || size % 2 != 0)
            throw new InvalidOperationException("Cannot resolve the physical controller's Windows instance. Reconnect it and retry.");
        string instance = Encoding.Unicode.GetString(data, 0, (int)size).TrimEnd('\0');
        if (!instance.StartsWith(@"HID\", StringComparison.OrdinalIgnoreCase) || instance.Length > 200 || instance.Contains('\0'))
            throw new InvalidOperationException("Invalid physical controller instance.");
        return instance;
    }
    public static string ImagePath(string executable)
    {
        string path = Path.GetFullPath(executable);
        if (path.Length < 3 || path[1] != ':') throw new InvalidOperationException("StageBox must run from a local drive for HidHide.");
        var target = new StringBuilder(32768);
        if (QueryDosDeviceW(path[..2], target, target.Capacity) == 0)
            throw new InvalidOperationException("Cannot resolve StageBox's image path for HidHide.");
        return target.ToString().Split('\0')[0] + path[2..];
    }
    public static void VerifyHidden(string path)
    {
        // This helper is deliberately NOT allowlisted. Only StageBox feeds SDL.
        using var probe = CreateFileW(path, 0, 3, IntPtr.Zero, 3, 0x80, IntPtr.Zero);
        if (!probe.IsInvalid)
            throw new InvalidOperationException("HidHide is not blocking the physical controller yet. Reconnect it or restart Windows, then retry. Also remove StageBox.HIDMaestro.exe from HidHide's application allowlist if it is listed.");
        int error = Marshal.GetLastWin32Error();
        if (error != 5)
            throw new InvalidOperationException($"Cannot verify HidHide protection (Windows error {error}). Reconnect the controller and retry.");
    }
}

internal sealed class HidHideAccessException(int code) : Exception($"Cannot open HidHide (Windows error {code}). Close its configuration client, or restart Windows if the driver was just installed.")
{
    public int Code { get; } = code;
}

internal static class HidHideSetup
{
    public static void EnsureInstalled()
    {
        try { using var control = HidHideControl.Open(); return; }
        catch (HidHideAccessException error) when (error.Code is 2 or 3) { }
        using var existing = Registry.ClassesRoot.OpenSubKey(@"Installer\Dependencies\NSS.Drivers.HidHide.x64");
        if (existing != null)
            throw new InvalidOperationException("HidHide is already installed but its driver is unavailable. Restart Windows, then launch this game again.");
        throw new InvalidOperationException("HidHide is not installed. Run install-hidhide.ps1 as Administrator from the StageBox bundle, then restart Windows before launching this game.");
    }
}

// The released driver has persistent lists, not the unreleased session API.
// Record our additions BEFORE any mutation; merge-only rollback preserves
// other tools' configuration and can recover after a helper crash/reboot.
internal sealed class HidHideLease : IDisposable
{
    internal sealed record Journal(int Version, int OwnerPID, long OwnerCreated, string Device, bool AddedDevice, string App, bool AddedApp, bool EnabledCloak,
        string[]? Devices = null, string[]? AddedDevices = null, string[]? ExistingDevices = null);
    private static readonly string JournalPath = Path.GetFullPath(Path.Combine(AppContext.BaseDirectory, "..", "hidhide-session.json"));
    private readonly Mutex ownership;
    private bool disposed;
    public string[] Instances { get; }

    private HidHideLease(Mutex mutex, string[] instances) { ownership = mutex; Instances = instances; }
    private static Mutex Lock()
    {
        var mutex = new Mutex(false, @"Global\StageBox.HidHide.Lease.v1");
        try {
            try { if (!mutex.WaitOne(0)) throw new InvalidOperationException("Another StageBox virtual-input session is using HidHide."); }
            catch (AbandonedMutexException) { } // Previous helper died: recover its journal.
            return mutex;
        } catch { mutex.Dispose(); throw; }
    }
    private static bool Alive(Journal journal)
    {
        try {
            using var owner = Process.GetProcessById(journal.OwnerPID);
            return !owner.HasExited && owner.StartTime.ToUniversalTime().ToFileTimeUtc() == journal.OwnerCreated;
        } catch (ArgumentException) { return false; }
    }
    private static Journal? ReadJournal()
    {
        if (!File.Exists(JournalPath)) return null;
        var file = new FileInfo(JournalPath);
        if (file.Length > 16384) throw new InvalidOperationException("Invalid HidHide recovery journal; review assets/hidhide-session.json.");
        var journal = JsonSerializer.Deserialize<Journal>(File.ReadAllBytes(JournalPath));
        if (journal == null || journal.Version is not (1 or 2) || journal.OwnerPID < 1 || journal.OwnerCreated <= 0 ||
            string.IsNullOrEmpty(journal.Device) || !journal.Device.StartsWith(@"HID\", StringComparison.OrdinalIgnoreCase) || journal.Device.Length > 200 || journal.Device.Contains('\0') ||
            string.IsNullOrEmpty(journal.App) || !journal.App.StartsWith(@"\Device\", StringComparison.OrdinalIgnoreCase) || journal.App.Length > 32768 || journal.App.Contains('\0'))
            throw new InvalidOperationException("Invalid HidHide recovery journal; review assets/hidhide-session.json.");
        if (journal.Version == 2 && (journal.Devices == null || journal.AddedDevices == null || journal.ExistingDevices == null ||
            journal.Devices.Length is < 1 or > 4 || journal.AddedDevices.Length > 4 || journal.ExistingDevices.Length > 4 ||
            journal.Devices.Concat(journal.AddedDevices).Concat(journal.ExistingDevices).Any(value => string.IsNullOrEmpty(value) || !value.StartsWith(@"HID\", StringComparison.OrdinalIgnoreCase) || value.Length > 200 || value.Contains('\0')) ||
            journal.AddedDevices.Any(value => !journal.Devices.Contains(value,StringComparer.OrdinalIgnoreCase))))
            throw new InvalidOperationException("Invalid HidHide recovery journal; review assets/hidhide-session.json.");
        return journal;
    }
    private static void Save(Journal journal)
    {
        string temp = JournalPath + "." + Guid.NewGuid().ToString("N") + ".tmp";
        try {
            using (var file = new FileStream(temp, FileMode.CreateNew, FileAccess.Write, FileShare.None, 4096, FileOptions.WriteThrough)) {
                JsonSerializer.Serialize(file, journal); file.Flush(true);
            }
            File.Move(temp, JournalPath, true);
        } finally { File.Delete(temp); }
    }
    private static void Restore(Journal journal)
    {
        using var driver = HidHideControl.Open();
        var devices = driver.List(2050);
        var additions = journal.AddedDevices ?? (journal.AddedDevice ? [journal.Device] : Array.Empty<string>());
        var original = journal.ExistingDevices ?? (journal.AddedDevice ? Array.Empty<string>() : [journal.Device]);
        if (additions.Length != 0) {
            devices = devices.Where(value => !additions.Contains(value,StringComparer.OrdinalIgnoreCase)).ToArray();
            driver.SetList(2051, devices);
        }
        // If another tool has added devices since we enabled cloaking, leave
        // its enable state intact. Never switch off somebody else's devices.
        if (journal.EnabledCloak && devices.All(value => original.Contains(value,StringComparer.OrdinalIgnoreCase)))
            driver.SetFlag(2053, false);
        if (journal.AddedApp) driver.SetList(2049, driver.List(2048).Where(value => !value.Equals(journal.App, StringComparison.OrdinalIgnoreCase)));
        File.Delete(JournalPath); // Keep recovery state if any driver write failed.
    }
    private static void RecoverLocked()
    {
        var journal = ReadJournal();
        if (journal == null) return;
        if (Alive(journal)) throw new InvalidOperationException("A live StageBox helper owns the HidHide session; its settings were preserved.");
        Restore(journal);
    }
    public static void RecoverStale()
    {
        if (!File.Exists(JournalPath)) return;
        using var mutex = Lock();
        try { RecoverLocked(); } finally { mutex.ReleaseMutex(); }
    }
    public static HidHideLease Begin(string[] paths, string executable)
    {
        string[] instances = paths.Select(HidHideControl.InstanceID).ToArray();
        if (instances.Length is < 1 or > 4 || instances.Distinct(StringComparer.OrdinalIgnoreCase).Count() != instances.Length)
            throw new InvalidOperationException("Physical controller instances must be distinct.");
        string app = HidHideControl.ImagePath(executable);
        var mutex = Lock();
        var lease = new HidHideLease(mutex, instances);
        try {
            RecoverLocked();
            using (var driver = HidHideControl.Open()) {
                if (driver.Flag(2054)) throw new InvalidOperationException("HidHide uses an inverted application list. Turn off inverse application cloak in HidHide before using StageBox virtual input.");
                var devices = driver.List(2050); var apps = driver.List(2048);
                bool active = driver.Flag(2052);
                // Enabling a previously disabled, nonempty list would also hide
                // unrelated devices. Require the user to resolve that conflict.
                if (!active && devices.Any(value => !instances.Contains(value,StringComparer.OrdinalIgnoreCase)))
                    throw new InvalidOperationException("HidHide has other devices configured while hiding is disabled. Enable hiding in HidHide or remove unused entries before retrying.");
                using var owner = Process.GetCurrentProcess();
                var additions = instances.Where(value => !devices.Contains(value,StringComparer.OrdinalIgnoreCase)).ToArray();
                var journal = new Journal(2, owner.Id, owner.StartTime.ToUniversalTime().ToFileTimeUtc(), instances[0],
                    additions.Contains(instances[0],StringComparer.OrdinalIgnoreCase), app,
                    !apps.Contains(app, StringComparer.OrdinalIgnoreCase), !active, instances, additions, active ? [] : devices);
                Save(journal);
                if (journal.AddedApp) driver.SetList(2049, apps.Append(app));
                if (additions.Length != 0) driver.SetList(2051, devices.Concat(additions));
                if (journal.EnabledCloak) driver.SetFlag(2053, true);
            }
            foreach (string path in paths) HidHideControl.VerifyHidden(path);
            return lease;
        } catch {
            lease.Dispose();
            throw;
        }
    }
    public void Dispose()
    {
        if (disposed) return;
        disposed = true;
        try {
            var journal = ReadJournal();
            if (journal != null) {
                using var owner = Process.GetCurrentProcess();
                if (journal.OwnerPID == owner.Id && journal.OwnerCreated == owner.StartTime.ToUniversalTime().ToFileTimeUtc()) Restore(journal);
            }
        } finally { ownership.ReleaseMutex(); ownership.Dispose(); }
    }
}
