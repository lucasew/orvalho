package runtime

const Polyfills = `
(function() {
    // Callback Manager
    globalThis.__go_callbacks = {};
    globalThis.__go_callback_id = 1;

    globalThis.__register_callback = function(fn) {
        const id = globalThis.__go_callback_id++;
        globalThis.__go_callbacks[id] = fn;
        return id;
    };

    globalThis.__unregister_callback = function(id) {
        delete globalThis.__go_callbacks[id];
    };

    globalThis.__invoke_callback = function(id, ...args) {
        const fn = globalThis.__go_callbacks[id];
        if (fn) {
            return fn(...args);
        }
    };

    // Timers
    globalThis.setTimeout = function(callback, delay) {
        const id = globalThis.__register_callback(callback);
        _go_setTimeout(id, delay);
        return id;
    };

    globalThis.clearTimeout = function(id) {
        _go_clearTimeout(id);
        globalThis.__unregister_callback(id);
    };

    globalThis.setInterval = function(callback, delay) {
        const id = globalThis.__register_callback(callback);
        _go_setInterval(id, delay);
        return id;
    };

    globalThis.clearInterval = function(id) {
        _go_clearInterval(id);
        globalThis.__unregister_callback(id);
    };

    // Fetch
    globalThis.fetch = function(url, options) {
        return new Promise((resolve, reject) => {
            const resolveId = globalThis.__register_callback(resolve);
            const rejectId = globalThis.__register_callback(reject);
            const optsStr = options ? JSON.stringify(options) : "{}";
            _go_fetch(url, optsStr, resolveId, rejectId);
        });
    };

    // TextEncoder/Decoder (Minimal Polyfill)
    if (typeof TextEncoder === "undefined") {
        globalThis.TextEncoder = class TextEncoder {
            encode(str) {
                const len = str.length;
                const res = new Uint8Array(len); // This is naive, only for ASCII/Latin1 mostly
                for (let i = 0; i < len; i++) {
                    res[i] = str.charCodeAt(i);
                }
                return res;
            }
        };
    }

    if (typeof TextDecoder === "undefined") {
        globalThis.TextDecoder = class TextDecoder {
            decode(arr) {
                return String.fromCharCode.apply(null, arr);
            }
        };
    }

    // Console
    if (typeof console === "undefined") {
        globalThis.console = {
            log: function(...args) { _go_print("LOG", ...args); },
            error: function(...args) { _go_print("ERROR", ...args); },
            warn: function(...args) { _go_print("WARN", ...args); },
            info: function(...args) { _go_print("INFO", ...args); }
        };
    }

    // Base64
    if (typeof btoa === "undefined") {
        globalThis.btoa = function(str) { return _go_btoa(str); }
    }
    if (typeof atob === "undefined") {
        globalThis.atob = function(str) { return _go_atob(str); }
    }
})();
`
