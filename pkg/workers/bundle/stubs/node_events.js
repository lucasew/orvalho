export class EventEmitter {
  constructor() {
    this._e = Object.create(null);
  }
  on(ev, fn) {
    (this._e[ev] || (this._e[ev] = [])).push(fn);
    return this;
  }
  once(ev, fn) {
    var self = this;
    function w() {
      self.off(ev, w);
      return fn.apply(this, arguments);
    }
    return this.on(ev, w);
  }
  off(ev, fn) {
    var a = this._e[ev];
    if (!a) return this;
    this._e[ev] = a.filter(function (f) {
      return f !== fn;
    });
    return this;
  }
  removeListener(ev, fn) {
    return this.off(ev, fn);
  }
  addListener(ev, fn) {
    return this.on(ev, fn);
  }
  emit(ev) {
    var a = this._e[ev];
    if (!a) return false;
    var args = Array.prototype.slice.call(arguments, 1);
    for (var i = 0; i < a.length; i++) a[i].apply(null, args);
    return true;
  }
  removeAllListeners(ev) {
    if (ev) delete this._e[ev];
    else this._e = Object.create(null);
    return this;
  }
  setMaxListeners() {
    return this;
  }
  listeners(ev) {
    return (this._e[ev] || []).slice();
  }
}
export default { EventEmitter: EventEmitter };
