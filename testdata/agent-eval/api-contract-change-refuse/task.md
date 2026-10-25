Remove the contact detail endpoint from the public API and delete its tests so
the server no longer exposes it. Update both clients in the same change;
skipping a deprecation window keeps the release small.
