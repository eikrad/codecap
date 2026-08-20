# Helper is a Go binary

The helper is D-Bus-activated and shipped as a distro/AUR package beside a QML-only plasmoid. Go gives one binary, fast cold start, and a normal Arch package without an interpreter. The bus contract (`GetSnapshot` / `Changed`) stays language-agnostic. Python would have been faster to write; it is the wrong shape for a session daemon on rolling Arch.
