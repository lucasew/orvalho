import { EventEmitter } from "node:events";
export class Writable extends EventEmitter {
  constructor() {
    super();
    this.writable = true;
  }
  write(chunk, enc, cb) {
    if (typeof enc === "function") cb = enc;
    if (cb) cb();
    return true;
  }
  end(chunk, enc, cb) {
    if (typeof chunk === "function") {
      cb = chunk;
      chunk = undefined;
    }
    if (chunk !== undefined) this.write(chunk);
    if (typeof enc === "function") cb = enc;
    if (cb) cb();
    this.emit("finish");
    return this;
  }
  cork() {}
  uncork() {}
  destroy() {
    this.emit("close");
    return this;
  }
}
export class Readable extends EventEmitter {
  constructor() {
    super();
    this.readable = true;
  }
  pipe() {
    return this;
  }
  read() {
    return null;
  }
}
export class Transform extends Writable {}
export class PassThrough extends Transform {}
export default {
  Writable: Writable,
  Readable: Readable,
  Transform: Transform,
  PassThrough: PassThrough,
};
