# Do not read these files directly

Use the `docket` CLI instead. Direct reads miss computed fields (AC status, linked files, state history).

```
docket list --state open --format context   # list open tickets
docket show TKT-NNN --format context        # read a specific ticket
```
