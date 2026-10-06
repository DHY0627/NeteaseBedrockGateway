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

var found = [];
Process.enumerateModules().forEach(function(m) {
    try {
        var exps = m.enumerateExports();
        exps.forEach(function(e) {
            if (e.name.toLowerCase().indexOf("chacha") !== -1) {
                found.push(m.name + "!" + e.name + " @" + e.address);
            }
        });
    } catch (err) {}
});
console.log("chacha exports:");
console.log(found.join("\n") || "(none)");

found.forEach(function(f) {
    if (f.indexOf("CRYPTO_chacha_20") !== -1) {
        var parts = f.split("!");
        var mname = parts[0];
        var ename = parts[1].split(" @")[0];
        var mod = Process.findModuleByName(mname);
        var target = mod.getExportByName(ename);
        console.log("hooking " + mname + "!" + ename);
        Interceptor.attach(target, {
            onEnter: function(args) {
                this.out = args[0];
                this.inp = args[1];
                this.len = args[2].toInt32();
                this.key = args[3];
                this.iv = args[4];
                this.counter = args[5].toInt32();
                console.log("[chacha] len=" + this.len + " counter=" + this.counter +
                    " key=" + hx(this.key, 32) + " iv=" + hx(this.iv, 16));
                if (this.len > 0 && this.len <= 4096) {
                    console.log("[chacha] in=" + hx(this.inp, Math.min(this.len, 96)));
                }
            },
            onLeave: function(retval) {
                if (this.len > 0 && this.len <= 4096) {
                    console.log("[chacha] out=" + hx(this.out, Math.min(this.len, 96)));
                }
            }
        });
    }
});
''')
script.on('message', lambda m, d: print(m.get('payload') if m.get('type') == 'send' else m))
script.load()
print("hook installed, listening...")
sys.stdout.flush()
time.sleep(3600)
