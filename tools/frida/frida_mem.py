import frida
import sys
import time

dev = frida.get_device_manager().add_remote_device('127.0.0.1:27042')
session = dev.attach(15956)
script = session.create_script(r'''
// 扫描内存找 "235847" 或房间号 ASCII，及其附近数据（TanCreateRoomRequest 明文）
var needles = ["235847", "361079"];
var found = 0;
Process.enumerateRanges('r--').forEach(function(range) {
    if (range.size > 256 * 1024 * 1024) return;
    try {
        var buf = Memory.readByteArray(range.base, Math.min(range.size, 32 * 1024 * 1024));
        if (!buf) return;
        var arr = new Uint8Array(buf);
        needles.forEach(function(needle) {
            var nb = [];
            for (var i = 0; i < needle.length; i++) nb.push(needle.charCodeAt(i));
            for (var i = 0; i < arr.length - nb.length; i++) {
                var match = true;
                for (var j = 0; j < nb.length; j++) {
                    if (arr[i + j] !== nb[j]) { match = false; break; }
                }
                if (match && found < 30) {
                    var start = Math.max(0, i - 60);
                    var ctx = [];
                    for (var k = start; k < Math.min(arr.length, i + 60); k++) {
                        ctx.push(arr[k].toString(16).padStart(2, "0"));
                    }
                    console.log("[hit@" + range.base.add(i) + "] " + ctx.join(" "));
                    found++;
                }
            }
        });
    } catch (e) {}
});
console.log("hits: " + found);
''')
script.on('message', lambda m, d: print(m.get('payload') if m.get('type') == 'send' else m))
script.load()
time.sleep(25)
session.detach()
print("done")
