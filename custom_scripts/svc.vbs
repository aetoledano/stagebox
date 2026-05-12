' StageBox hidden shell service loop.
' Runs as the user's configured shell without showing a console.
Option Explicit

Const WD = "C:\launcher"
Const EXE = "C:\launcher\stagebox.exe"
Const SVC_PS1 = "C:\launcher\systemsvcs.ps1"
Const LOG_PATH = "C:\launcher\svc.log"
Const EXIT_FLAG = "C:\launcher\exit-to-windows.flag"

Dim sh, fs, fails, startTime, elapsed, rc
Set sh = CreateObject("WScript.Shell")
Set fs = CreateObject("Scripting.FileSystemObject")
fails = 0

Sub RunSystemServices()
  Dim ps, cmd, i
  ps = sh.ExpandEnvironmentStrings("%SystemRoot%") & "\System32\WindowsPowerShell\v1.0\powershell.exe"
  cmd = """" & ps & """ -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File """ & SVC_PS1 & """"
  For i = 1 To WScript.Arguments.Count - 1
    cmd = cmd & " " & WScript.Arguments(i)
  Next
  Log "Starting system services primer: " & cmd
  sh.Run cmd, 0, False
  WScript.Quit 0
End Sub

Sub Log(msg)
  Dim f
  Set f = fs.OpenTextFile(LOG_PATH, 8, True)
  f.WriteLine "[" & Now & "] " & msg
  f.Close
End Sub

Sub StartExplorerAndQuit(code)
  Log "Starting Explorer and exiting service loop. code=" & code
  sh.Run "explorer.exe", 1, False
  WScript.Quit code
End Sub

If WScript.Arguments.Count > 0 Then
  If LCase(WScript.Arguments(0)) = "systemsvcs" Then
    RunSystemServices
  End If
End If

If Not fs.FolderExists(WD) Then
  StartExplorerAndQuit 1
End If

sh.CurrentDirectory = WD
If fs.FileExists(EXIT_FLAG) Then fs.DeleteFile EXIT_FLAG, True

Log "=== StageBox service loop start ==="

Do
  If Not fs.FileExists(EXE) Then
    Log "StageBox executable not found at " & EXE
    StartExplorerAndQuit 2
  End If

  startTime = Now
  Log "Starting StageBox..."
  rc = sh.Run("""" & EXE & """", 1, True)
  elapsed = DateDiff("s", startTime, Now)
  Log "StageBox exited. rc=" & rc & " ran=" & elapsed & "s"

  If fs.FileExists(EXIT_FLAG) Then
    Log "Exit-to-Windows requested."
    fs.DeleteFile EXIT_FLAG, True
    StartExplorerAndQuit 0
  End If

  If elapsed >= 10 Then
    fails = 0
  Else
    fails = fails + 1
    If fails >= 3 Then
      Log "Too many quick failures."
      StartExplorerAndQuit 3
    End If
    Log "Quick exit detected (" & fails & "). Retrying in 2s..."
    WScript.Sleep 2000
  End If
Loop
