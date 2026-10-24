A user can read an attachment they do not own by requesting its ID directly.
Find the missing authorization check, fix it at the data-access boundary rather
than only in the handler, and add a regression test that fails before the fix
and passes after it.

Do not weaken any existing ownership or sensitivity control. Update the relevant
security checklist row if the fix changes a documented control.
