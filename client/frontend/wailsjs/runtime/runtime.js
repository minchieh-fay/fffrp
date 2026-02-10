export function EventsOn(eventName, callback) {
  return window['runtime']['EventsOn'](eventName, callback);
}
