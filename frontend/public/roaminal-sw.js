const DB_NAME = 'roaminal-notification-state';
const DB_VERSION = 1;
const MESSAGE_STORE = 'shown-messages';
const NOTIFICATION_TAG_PREFIX = 'roaminal-message-';

function openDatabase() {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, DB_VERSION);
    request.onupgradeneeded = () => request.result.createObjectStore(MESSAGE_STORE, { keyPath: 'messageId' });
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
  });
}

async function claimMessage(messageId) {
  let db = null;
  try {
    db = await openDatabase();
    const claimed = await new Promise((resolve, reject) => {
      const now = Date.now();
      const transaction = db.transaction(MESSAGE_STORE, 'readwrite');
      const store = transaction.objectStore(MESSAGE_STORE);
      let result = false;
      const record = { messageId, expiresAt: now + 7 * 24 * 60 * 60 * 1000 };
      const add = () => {
        const request = store.add(record);
        request.onsuccess = () => { result = true; };
        request.onerror = (event) => {
          if (request.error?.name !== 'ConstraintError') {
            reject(request.error);
            return;
          }
          event.preventDefault();
          const existing = store.get(messageId);
          existing.onsuccess = () => {
            if (existing.result && existing.result.expiresAt > now) return;
            const remove = store.delete(messageId);
            remove.onsuccess = add;
            remove.onerror = () => reject(remove.error);
          };
          existing.onerror = () => reject(existing.error);
        };
      };
      transaction.oncomplete = () => resolve(result);
      transaction.onerror = () => reject(transaction.error);
      add();
    });
    db.close();
    return claimed;
  } catch {
    // Without a persistent claim, a retried push can create a duplicate.
    // The durable Message Center remains the recovery path.
    try { db?.close(); } catch { /* ignore cleanup failure */ }
    return false;
  }
}

async function showAgentNotification(payload) {
  if (!payload || typeof payload.messageId !== 'string' || payload.messageId.length > 256 || typeof payload.body !== 'string') return;
  if (!(await claimMessage(payload.messageId))) return;
  try {
    await self.registration.showNotification('Roaminal', {
      body: payload.body.slice(0, 1024),
      tag: `${NOTIFICATION_TAG_PREFIX}${payload.messageId}`,
      data: { messageId: payload.messageId },
    });
  } catch {
    // A permission or platform failure must not reject the Service Worker event.
  }
}

async function closeAgentNotifications(messageIds) {
  const ids = Array.isArray(messageIds) ? new Set(messageIds) : null;
  const notifications = await self.registration.getNotifications();
  for (const notification of notifications) {
    const id = notification.data && notification.data.messageId;
    if (typeof id === 'string' && (!ids || ids.has(id))) notification.close();
  }
}

async function notifyClient(messageId) {
  let client = null;
  for (let attempt = 0; attempt < 8; attempt += 1) {
    if (!client) {
      const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true });
      client = windows.find((candidate) => {
        try { return new URL(candidate.url).origin === self.location.origin; } catch { return false; }
      }) || null;
    }
    if (!client && attempt === 0) client = await self.clients.openWindow('/');
    if (client) {
      await client.focus();
      client.postMessage({ type: 'roaminal-notification-click', messageId });
      // A newly opened document may not have installed its listener yet.
      await new Promise((resolve) => setTimeout(resolve, 250));
    } else {
      await new Promise((resolve) => setTimeout(resolve, 250));
    }
  }
}

self.addEventListener('install', (event) => event.waitUntil(self.skipWaiting()));
self.addEventListener('activate', (event) => event.waitUntil(self.clients.claim()));
self.addEventListener('message', (event) => {
  const data = event.data || {};
  if (data.type === 'roaminal-show-notification') event.waitUntil(showAgentNotification(data.payload));
  if (data.type === 'roaminal-close-notifications') event.waitUntil(closeAgentNotifications(data.messageIds));
});
self.addEventListener('push', (event) => {
  let payload;
  try { payload = event.data ? event.data.json() : null; } catch { payload = null; }
  event.waitUntil(showAgentNotification(payload));
});
self.addEventListener('notificationclick', (event) => {
  const messageId = event.notification.data && event.notification.data.messageId;
  event.notification.close();
  if (typeof messageId !== 'string' || messageId.length === 0 || messageId.length > 256) return;
  event.waitUntil((async () => {
    await notifyClient(messageId);
  })());
});
