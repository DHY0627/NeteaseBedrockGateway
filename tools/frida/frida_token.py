import frida
import sys
import time

dev = frida.get_device_manager().add_remote_device('127.0.0.1:27042')
session = dev.attach(15956)
script = session.create_script(r'''
// 在可读内存中搜索 ASCII 字符串 "sauth_json"、"login_token"、"user_token" 附近内容
function scanAscii() {
    var patterns = ["sauth_json", "login_token", "user_token", "userToken", "token"];
    var results = {};
    Process.enumerateRanges('r--').forEach(function(range) {
        if (range.size > 512 * 1024 * 1024) return;
        try {
            var bytes = Memory.readByteArray(range.base, Math.min(range.size, 32 * 1024 * 1024));
            if (!bytes) return;
            var arr = new Uint8Array(bytes);
            var s = "";
            var start = 0;
            for (var i = 0; i < arr.length; i++) {
                var c = arr[i];
                if (c >= 32 && c < 127) {
                    if (s.length === 0) start = i;
                    s += String.fromCharCode(c);
                } else {
                    if (s.length >= 6 && s.length <= 512) {
                        patterns.forEach(function(p) {
                            if (s.indexOf(p) !== -1) {
                                var key = p + ":" + s;
                                if (!results[key]) {
                                    results[key] = range.base.add(start);
                                    console.log("[" + p + "] @" + range.base.add(start) + ": " + s);
                                }
                            }
                        });
                    }
                    s = "";
                }
                if (s.length > 1024) s = "";
            }
        } catch (e) {}
    });
}
scanAscii();
''')
script.on('message', lambda m, d: print(m.get('payload') if m.get('type') == 'send' else m))
script.load()
time.sleep(20)
session.detach()
print("done")
