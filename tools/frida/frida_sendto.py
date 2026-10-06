import frida
import sys
import time

dev = frida.get_device_manager().add_remote_device('127.0.0.1:27042')
session = dev.attach(15956)
script = session.create_script(r'''
function hx(ptr, len) {
    var out = [];
    for (var i = 0; i < len; i++) out.push(ptr.add(i).readU8().toString(16).padStart(2, "0"));
    return out.join("");
}
var target = null;
Process.enumerateModules().forEach(function(m) {
    if (m.name === "libc.so" || m.name === "libc.so.6") {
        try {
            var e = m.getExportByName("sendto");
            if (e) target = e;
        } catch (err) {}
    }
});
if (!target) {
    // 备用: 直接搜索符号
    var mods = Process.enumerateModules();
    for (var i = 0; i < mods.length; i++) {
        try {
            var exps = mods[i].enumerateExports();
            for (var j = 0; j < exps.length; j++) {
                if (exps[j].name === "sendto") { target = exps[j].address; break; }
            }
        } catch (e) {}
        if (target) break;
    }
}
console.log("sendto at " + target);
if (target) {
    Interceptor.attach(target, {
        onEnter: function(args) {
            this.buf = args[1];
            this.len = args[2].toInt32();
            this.addr = args[4];
            try {
                var family = this.addr.readU16();
                if (family === 2) {
                    var port = (this.addr.add(2).readU8() << 8) | this.addr.add(3).readU8();
                    if ((port >= 10000 && port <= 10100) || port === 8899 || port === 8900) {
                        console.log("[sendto] len=" + this.len + " port=" + port +
                            " data=" + hx(this.buf, Math.min(this.len, 240)));
                    }
                }
            } catch (e) {}
        }
    });
    console.log("hooked sendto");
} else {
    console.log("sendto NOT found");
}
''')
script.on('message', lambda m, d: print(m.get('payload') if m.get('type') == 'send' else m))
script.load()
print("listening...")
sys.stdout.flush()
time.sleep(600)
