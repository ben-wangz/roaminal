export function notify(title: string, body: string): void { if ('Notification' in window && Notification.permission === 'granted') new Notification(title, { body }); }
