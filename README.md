# Alogview - a coloured log viewer for ADB logcat

`Alogview` is a simple wrapper for `adb logcat`,
which shows all log messages originating from specific application packages.
It also colours them according to their severity level.

It is inspired by Jake Whartons excellent [`pidcat`](https://github.com/JakeWharton/pidcat) script,
which uses its own output format,
which I find inconvenient.

## Usage

`alogview [PACKAGES]`

Invoking `alogview` without any argument prints all received log messages to stdout.
If one or more `package` is supplied,
only log messages emitted by processes belonging to those packages are printed.

## Installation

`Adb` from the Android SDK is expected to be on the `PATH`.

A simple `go get github.com/flimberger/alogview` is sufficient.
`Alogview` does not have any further dependencies.
