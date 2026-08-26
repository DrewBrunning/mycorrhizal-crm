// Browser Web Push subscription helper (N9). Turns the server's VAPID public
// key into a browser PushSubscription and back into the API's wire shape.
import type { PushSubscriptionInput } from './api/notifications';

// urlBase64ToUint8Array converts a base64url-encoded VAPID public key into the
// Uint8Array the Push API expects for applicationServerKey.
export function urlBase64ToUint8Array(base64String: string): Uint8Array<ArrayBuffer> {
  const padding = '='.repeat((4 - (base64String.length % 4)) % 4);
  const base64 = (base64String + padding).replace(/-/g, '+').replace(/_/g, '/');
  const rawData = window.atob(base64);
  const outputArray = new Uint8Array(new ArrayBuffer(rawData.length));
  for (let i = 0; i < rawData.length; ++i) {
    outputArray[i] = rawData.charCodeAt(i);
  }
  return outputArray;
}

// browserSupportsPush reports whether the current browser can subscribe.
export function browserSupportsPush(): boolean {
  return 'serviceWorker' in navigator && 'PushManager' in window;
}

// requestPushPermission asks for the notification permission the Push API
// requires, returning true when granted.
export async function requestPushPermission(): Promise<boolean> {
  if (!('Notification' in window)) {
    return false;
  }
  if (Notification.permission === 'granted') {
    return true;
  }
  if (Notification.permission === 'denied') {
    return false;
  }
  return (await Notification.requestPermission()) === 'granted';
}

// subscribeBrowserPush registers this browser with the push service using the
// server's VAPID public key and returns the subscription in the API's wire
// shape, ready for POST /notifications/push-subscriptions.
export async function subscribeBrowserPush(vapidPublicKey: string): Promise<PushSubscriptionInput> {
  if (!browserSupportsPush()) {
    throw new Error('push unsupported');
  }
  if (!(await requestPushPermission())) {
    throw new Error('push denied');
  }

  let registration = await navigator.serviceWorker.getRegistration();
  if (!registration) {
    // index.tsx registers the app's service worker (precache + push handler) on
    // window load, so in production one normally exists by now. This covers the
    // race where the user reaches Settings before that ran -- and it is the
    // only path that registers at all in a dev build, where register() is a
    // no-op.
    //
    // Note this is a fallback, not the mechanism: until 2026-08-06 index.tsx
    // called unregister(), and relying on this line alone meant the
    // registration -- and the subscription created below, which the
    // registration owns -- was destroyed on the next page load.
    const swUrl = `${import.meta.env.BASE_URL}service-worker.js`;
    registration = await navigator.serviceWorker.register(swUrl);
  }

  const subscription = await registration.pushManager.subscribe({
    userVisibleOnly: true,
    applicationServerKey: urlBase64ToUint8Array(vapidPublicKey),
  });

  const raw = subscription.toJSON();
  const keys = raw.keys || {};
  return {
    endpoint: subscription.endpoint,
    p256dh: keys.p256dh || '',
    auth: keys.auth || '',
    device_label: deviceLabel(),
  };
}

// deviceLabel produces a short human-readable name for the device being
// registered (shown in the Settings device list).
function deviceLabel(): string {
  const ua = typeof navigator !== 'undefined' ? navigator.userAgent : '';
  if (/Firefox\//.test(ua)) return 'Firefox';
  if (/Chrome\//.test(ua) && !/Edg\//.test(ua)) return 'Chrome';
  if (/Edg\//.test(ua)) return 'Edge';
  if (/Safari\//.test(ua) && !/Chrome\//.test(ua)) return 'Safari';
  if (/Mobile|Android|iPhone/.test(ua)) return 'Mobile';
  return 'Browser';
}
