-- Reverses 000019. Destructive by nature: every registered mobile device
-- token is discarded. Down is the only lossy direction — a device that
-- re-registers after a rollback simply calls POST /notifications/devices
-- again, which is exactly what it does on every app launch.

DROP TABLE device_registrations;
