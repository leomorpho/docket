# UI Mutation QA Checklist

## Preconditions
- Run `docket ui` from repo root.
- Confirm prompt reads `[Y/n]` and pressing `Enter` opens browser.
- Use a test ticket in an open state (for example `backlog` or `todo`).

## Happy Paths
1. Open a ticket from board or list.
2. Change state in the detail sheet and click `Update state`.
3. Confirm success message appears and ticket state updates in board/list after refresh.
4. Edit title and click `Save title`.
5. Confirm success message appears and title updates in board/list card and detail header.
6. Edit description and click `Save description`.
7. Confirm success message appears and markdown section refreshes with new description.

## Failure Paths
1. Try saving an empty title.
2. Confirm client validation blocks submission and shows a clear error.
3. Force a command failure (for example invalid state via API call) and verify UI shows returned error message.

## Consistency Checks
- While a save is running, the related save button is disabled.
- Repeated clicks during save do not trigger duplicate updates.
- Closing and reopening the sheet shows the latest persisted ticket values.
