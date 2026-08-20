# Helper lives for the graphical session

D-Bus activation starts the Go helper on first GetSnapshot. Idle-exit would drop file-watch and Allowance polling until the next call. v1 keeps the process up until logout; systemd --user restarts it on crash. A small daemon is cheaper than reconnecting OAuth and logs every idle timeout.
