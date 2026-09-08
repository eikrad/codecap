# QML handlers read raw inputs, never derived values

Both halves of the applet's D-Bus path were broken by a handler trusting a value it should have derived itself, and neither could be seen without executing the code.

`refresh()` tested `isBound`, a binding on `accountHome`. QML bindings are push-based, so when `refresh()` ran from `onAccountHomeChanged` the binding had not been re-evaluated and `isBound` was still `false`: binding an Account Home sent no `GetSnapshot` at all. `dbusChanged` compared its argument to `accountHome` with `===`. `SignalWatcher` decodes a `"s"` argument to `{value: "..."}`, so the comparison was false for every `Changed` the helper ever sent, and ADR 0011's primary update trigger has never fired — only the 30 s safety-net poll.

A handler therefore reads the property it is a handler for, or the raw argument it was given, and computes anything else from those. It does not read a binding that depends on what just changed, and it does not assume a platform hands it the JavaScript type the signature implies. Decoding belongs in `logic.js`, where `extractSnapshotPayload` and `signalAccountHome` sit next to Node tests over shapes captured from a real bus.

Neither failure is visible to `qmllint`, which cannot resolve Plasma types, or to the Node tests, which have no bindings and no bus. `tests/plasmoid/qml-dbus` is the gate: it was 3 of 10 the first time it ran.
