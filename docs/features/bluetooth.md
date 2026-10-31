# Bluetooth devices

[Documentation](../README.md) · [StageBox](../../README.md)

Use **Tools → Bluetooth Devices → Pair device** to open Windows pairing. Put the controller in its pairing mode and follow the Windows wizard. StageBox returns to the launcher afterward.

The topbar Bluetooth icon is green when a controller is connected directly through Windows Bluetooth. A remembered pairing alone does not turn it green; a USB wireless receiver is a different connection type.

## Forget a paired device

Open **Tools → Bluetooth Devices**, select the device by name/address and choose **Forget device**. Up/Down moves through the list; Left/Right moves between actions. Cancel returns to the same device. This removes its Windows pairing, and a connected device disconnects. Keep another controller, keyboard or mouse available. Use **Pair device** afterward to open the existing Windows pairing wizard.

The list covers Classic and Bluetooth LE devices paired directly with Windows, including the Ultimate 2C over direct Bluetooth LE. LE rows are labeled **Bluetooth LE**. A paired LE device can remain listed while disconnected even if Windows omits its address; it shows **Address unavailable**. If one Bluetooth backend fails, available devices remain visible with a warning. Pairings stored by USB adapters, including 8BitDo adapters, are managed by the adapter and are not Windows pairing records. Forgetting a Windows pairing also removes saved profiles linked to that device from `assets/controllers.json`. If Windows supplies no address to link the pairing to its profile, the device is still forgotten and StageBox reports that saved-profile cleanup could not be verified; remove that stale entry through Controllers after unpairing. Shared SDL mapping overrides remain available. Windows PowerShell hosts the built-in Windows Runtime API for LE enumeration/unpairing; its script is embedded in StageBox, with no extra helper file to copy.
