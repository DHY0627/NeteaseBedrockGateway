import frida
import sys

dev = frida.get_device_manager().add_remote_device('127.0.0.1:27042')
session = dev.attach(15956)
script = session.create_script(r'''
console.log("attached ok");
var mods = Process.enumerateModules().map(function(m) { return m.name; });
console.log("modules: " + mods.length);
''')
script.on('message', lambda m, d: print(m.get('payload') if m.get('type') == 'send' else m))
script.load()
import time
time.sleep(1)
session.detach()
print("detached cleanly")
