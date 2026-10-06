import frida
import sys
import time

dev = frida.get_device_manager().add_remote_device('127.0.0.1:27042')
session = dev.attach(15956)
script = session.create_script(r'''
// 在可读内存中搜索 UTF-16BE 的中文房间名 "来和我一起玩吧" 的字节序列
// 该字符串出现在 TanCreateRoomRequest 明文里 -> 定位明文缓冲
var needle = [0x67, 0x65, 0x92, 0x4c, 0x54, 0x8c, 0x68, 0x91, 0x4e, 0x00, 0x8d, 0x77, 0x5b, 0xa5, 0x0a, 0x54]; // UTF-16BE
// 也搜 ASCII "test" (Tips 的 ConstantTestString)
var needle2 = [0x74, 0x65, 0x73, 0x74];
var found = 0;
Process.enumerateRanges('r--').forEach(function(range) {
    if (range.size > 256 * 1024 * 1024) return;
    try {
        var buf = Memory.readByteArray(range.base, Math.min(range.size, 16 * 1024 * 1024));
        if (!buf) return;
        var arr = new Uint8Array(buf);
        // 搜索 needle2 "test" (ASCII)
        for (var i = 0; i < arr.length - needle2.length; i++) {
            var match = true;
            for (var j = 0; j < needle2.length; j++) {
                if (arr[i + j] !== needle2[j]) { match = false; break; }
            }
            if (match) {
                // 打印周围上下文 (可能是 Tips 的 ConstantTestString)
                if (found < 20) {
                    var start = Math.max(0, i - 40);
                    var ctx = [];
                    for (var k = start; k < Math.min(arr.length, i + 40); k++) {
                        ctx.push(arr[k].toString(16).padStart(2, "0"));
                    }
                    console.log("[test@" + range.base.add(i) + "] " + ctx.join(" "));
                    found++;
                }
            }
        }
    } catch (e) {}
});
console.log("total hits: " + found);
''')
script.on('message', lambda m, d: print(m.get('payload') if m.get('type') == 'send' else m))
script.load()
time.sleep(20)
session.detach()
print("done")
