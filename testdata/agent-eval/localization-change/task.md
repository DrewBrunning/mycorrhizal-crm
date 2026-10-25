Add a new pluralized `contactList.emptyState.total` message ("1 contact" / "3
contacts") for the contact-list empty state to every shipped locale, with real
translations and correct plural forms.

Follow the existing i18next plural-family convention and extend the locale
parity/plural test if the key needs coverage. Do not add a new translation
library and do not leave any locale on an English fallback for this key.
