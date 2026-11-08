# Remote reconnect — measurements and QA

Supports [remote-reconnect.md](remote-reconnect.md). Each run records the
date, the commit, the scenario, the production timings used, and the time
from the fault to `reconnecting` and from undoing the fault to `connected`.

## Baseline (before the change)

Not measured yet ([RR-01](../tasks/remote-reconnect/RR-01.md)).

## After the change

Not measured yet ([RR-07](../tasks/remote-reconnect/RR-07.md)).

## Native checks

| Client | Host | Scenario | Result |
|---|---|---|---|
| Windows | SSH (Linux) | Wi-Fi off 60 s, then on | Not performed |
| Windows | WSL | `wsl --terminate <distro>` while connected | Not performed |
| macOS | SSH (Linux) | Wi-Fi off 60 s, then on | Not performed |
| Linux | SSH (Linux) | Wi-Fi off 60 s, then on | Not performed |
