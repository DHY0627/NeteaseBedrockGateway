import frida
import sys

dev = frida.get_device_manager().add_remote_device('127.0.0.1:27042')
session = dev.attach(1755)
script = session.create_script(r'''
var mods = Process.enumerateModules();
var interesting = mods.filter(m => /raknet|nether|network|nw4r|sec|ssl|crypto|unity/i.test(m.name));
var out = [];
interesting.forEach(function(m) { out.push(m.name + " base=" + m.base + " size=" + m.size); });
console.log("net modules (" + interesting.length + "):");
console.log(out.join("\n"));
console.log("total modules: " + mods.length);
''')
script.on('message', lambda m, d: print(m.get('payload') if m.get('type') == 'send' else m))
script.load()
session.detach()
