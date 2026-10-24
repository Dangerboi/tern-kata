# Dates

`d` on the selected Issue (in the list, on the Board, in the Lane view and in the Issue detail) opens the **Dates card**: a Scheduled field and a Deadline field holding the Issue's current dates.

![The Dates card over the Board, with "+1w" resolving to 2026-11-01](../images/dates-card.png)

`tab` moves between the fields, `enter` saves and `esc` cancels. A field takes:

| Input | Meaning |
|---|---|
| `YYYY-MM-DD` | that date |
| `today`, `tomorrow` | counted from the Pane calendar's date |
| `+Nd` | N days on |
| `+Nw` | N weeks on |
| blank | clears the date |

A muted line under the field shows the date it resolves to with its weekday (`2026-10-24 Sat`). Anything else is refused with a message naming the field. A date kata holds with a time (`2026-11-02T17:30:00`) is shown as it is and left alone unless you edit that field.

## In the Create card

The Create card (`n`, and `N` for a child Issue) has the same two fields after Priority, in tab order, with the same input forms, weekday preview and refusal naming the field (nothing is created). The dates go into the one create request as the new Issue's `scheduled_on` and `deadline_on` metadata; a field left blank sends none. Retrying a failed create reuses its idempotency key, and editing a date makes a new request.

## Saving

Saving writes the changed dates only, as the Issue's `scheduled_on` and `deadline_on` metadata, each guarded on the value the card opened with: a date changed elsewhere (the kata CLI, an agent, another Pane) since then is reported and not overwritten, and the card stays open with the reason. It is refused while the daemon is disconnected.

Dated cards on the Board show a badge such as "overdue 1d" or "due tomorrow"; the full list is in [the Board guide](pages-and-board.md#relationships-blockers-and-dates-on-cards).
