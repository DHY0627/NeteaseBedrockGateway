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

// 1. 列出所有可能的网络/加密库
var mods = Process.enumerateModules();
var net = mods.filter(function(m) {
    return /net|rak|sock|ssl|crypto|minecraft|bedrock|game|mc/i.test(m.name) && m.size > 100000;
});
console.log("candidate modules:");
net.forEach(function(m) {
    console.log(m.name + " size=" + m.size + " base=" + m.base);
});

// 2. 在所有模块中搜索 chacha8/chacha 相关导出（非 OpenSSL 的）
var customChacha = [];
Process.enumerateModules().forEach(function(m) {
    try {
        m.enumerateExports().forEach(function(e) {
            var n = e.name.toLowerCase();
            if ((n.indexOf("chacha") !== -1 || n.indexOf("chacha8") !== -1 || n.indexOf("x19") !== -1) &&
                n.indexOf("poly1305") === -1 && n.indexOf("aead") === -1 && n.indexOf("hpke") === -1) {
                customChacha.push(m.name + "!" + e.name + " @" + e.address);
            }
        });
    } catch (err) {}
});
console.log("custom chacha exports:");
console.log(customChacha.join("\n") || "(none)");
''')
script.on('message', lambda m, d: print(m.get('payload') if m.get('type') == 'send' else m))
script.load()
time.sleep(3)
session.detach()
print("done")
