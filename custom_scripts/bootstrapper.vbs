' C:\launcher\bootstrapper.vbs
Option Explicit
Dim sh, cmd
Set sh = CreateObject("WScript.Shell")
sh.CurrentDirectory = "C:\launcher"
cmd = "cmd.exe /d /c ""C:\launcher\bootstrapper.cmd"""
' 0 = hidden window, True = wait (so this stays as the shell process)
sh.Run cmd, 0, True
