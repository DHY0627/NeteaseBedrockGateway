import frida
import sys
import time

dev = frida.get_device_manager().add_remote_device('127.0.0.1:27042')
session = dev.attach(15956)
script = session.create_script(r'''
// 通过 /proc/self/maps 找 libminecraftpe.so 基址
var maps = new File("/proc/self/maps", "r").read().split("\n");
var found = [];
maps.forEach(function(line) {
    var parts = line.split(/\s+/);
    if (parts.length >= 6 && /minecraftpe|EncryptorP/.test(parts[5])) {
        var addrs = parts[0].split("-");
        found.push({ name: parts[5], base: ptr("0x" + addrs[0]), size: ptr("0x" + addrs[1]).sub(ptr("0x" + addrs[0])) });
    }
});
found.forEach(function(m) {
    console.log("FOUND " + m.name + " base=" + m.base + " size=" + m.size);
});
if (found.length === 0) console.log("minecraftpe not in maps");
''')
script.on('message', lambda m, d: print(m.get('payload') if m.get('type') == 'send' else m))
script.load()
time.sleep(2)
session.detach()
print("done")
