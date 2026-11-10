using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Runtime.InteropServices;
using System.Text;

public static class NativeUI
{
    public sealed class Control
    {
        public IntPtr Handle { get; set; }
        public string ClassName { get; set; }
        public string Text { get; set; }
        public int Style { get; set; }
        public int Id { get; set; }
    }

    private delegate bool EnumWindow(IntPtr window, IntPtr parameter);
    [DllImport("user32.dll")] private static extern bool EnumWindows(EnumWindow callback, IntPtr parameter);
    [DllImport("user32.dll")] private static extern bool EnumChildWindows(IntPtr parent, EnumWindow callback, IntPtr parameter);
    [DllImport("user32.dll")] private static extern uint GetWindowThreadProcessId(IntPtr window, out int processId);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] private static extern int GetClassName(IntPtr window, StringBuilder name, int length);
    [DllImport("user32.dll", EntryPoint = "GetWindowLongW")] private static extern int GetWindowLong(IntPtr window, int index);
    [DllImport("user32.dll")] public static extern IntPtr GetParent(IntPtr window);
    [DllImport("user32.dll")] public static extern int GetDlgCtrlID(IntPtr window);
    [DllImport("user32.dll")] public static extern bool IsWindowEnabled(IntPtr window);
    [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr window);
    [DllImport("user32.dll", EntryPoint = "SendMessageTimeoutW", SetLastError = true)] private static extern IntPtr SendMessageTimeout(IntPtr window, uint message, IntPtr wParam, IntPtr lParam, uint flags, uint timeout, out IntPtr result);
    [DllImport("user32.dll", EntryPoint = "PostMessageW", SetLastError = true)] private static extern bool PostMessage(IntPtr window, uint message, IntPtr wParam, IntPtr lParam);

    public static IntPtr FindWindow(int processId, string title)
    {
        IntPtr found = IntPtr.Zero;
        EnumWindows(delegate(IntPtr window, IntPtr parameter) {
            int owner;
            GetWindowThreadProcessId(window, out owner);
            if (owner == processId && Text(window) == title && IsWindowVisible(window)) found = window;
            return true;
        }, IntPtr.Zero);
        return found;
    }

    public static Control[] Children(IntPtr parent)
    {
        var controls = new List<Control>();
        EnumChildWindows(parent, delegate(IntPtr window, IntPtr parameter) {
            var name = new StringBuilder(256);
            GetClassName(window, name, name.Capacity);
            controls.Add(new Control { Handle = window, ClassName = name.ToString(), Text = Text(window), Style = GetWindowLong(window, -16), Id = GetDlgCtrlID(window) });
            return true;
        }, IntPtr.Zero);
        return controls.ToArray();
    }

    public static string Text(IntPtr window)
    {
        int count = Send(window, 0x000E, IntPtr.Zero, IntPtr.Zero).ToInt32();
        if (count == 0) return "";
        IntPtr buffer = Marshal.AllocHGlobal((count + 1) * 2);
        try {
            int copied = Send(window, 0x000D, new IntPtr(count + 1), buffer).ToInt32();
            return Marshal.PtrToStringUni(buffer, copied);
        } finally {
            Marshal.FreeHGlobal(buffer);
        }
    }

    public static void SetText(IntPtr window, string value)
    {
        IntPtr buffer = Marshal.StringToHGlobalUni(value);
        try {
            if (Send(window, 0x000C, IntPtr.Zero, buffer) == IntPtr.Zero) throw new InvalidOperationException("无法填写控件");
        } finally {
            Marshal.FreeHGlobal(buffer);
        }
    }

    public static void Click(IntPtr window)
    {
        if (!PostMessage(window, 0x00F5, IntPtr.Zero, IntPtr.Zero)) throw new Win32Exception();
    }

    public static void Right(IntPtr window)
    {
        Send(window, 0x0100, new IntPtr(0x27), new IntPtr(1));
        Send(window, 0x0101, new IntPtr(0x27), new IntPtr(1));
    }

    public static int SelectedTab(IntPtr window)
    {
        return Send(window, 0x130B, IntPtr.Zero, IntPtr.Zero).ToInt32();
    }

    public static void Left(IntPtr window)
    {
        Send(window, 0x0100, new IntPtr(0x25), new IntPtr(1));
        Send(window, 0x0101, new IntPtr(0x25), new IntPtr(1));
    }

    public static void SelectList(IntPtr window, int index)
    {
        Send(window, 0x0186, new IntPtr(index), IntPtr.Zero);
        Send(GetParent(window), 0x0111, new IntPtr((1 << 16) | GetDlgCtrlID(window)), window);
    }

    public static void Close(IntPtr window)
    {
        Send(window, 0x0010, IntPtr.Zero, IntPtr.Zero);
    }

    private static IntPtr Send(IntPtr window, uint message, IntPtr wParam, IntPtr lParam)
    {
        IntPtr result;
        if (SendMessageTimeout(window, message, wParam, lParam, 2, 3000, out result) == IntPtr.Zero) throw new Win32Exception();
        return result;
    }
}
