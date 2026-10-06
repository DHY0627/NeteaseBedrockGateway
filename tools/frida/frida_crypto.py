import frida
import sys

dev = frida.get_device_manager().add_remote_device('127.0.0.1:27042')
session = dev.attach(1755)
script = session.create_script(r'''
var crypto = Process.findModuleByName("libcrypto.so");
if (!crypto) { console.log("libcrypto not found"); }
else {
    var names = crypto.enumerateExports().map(function(e) { return e.name; });
    var hits = names.filter(function(n) { return /chacha|ChaCha|EVP_Encrypt|EVP_Cipher|EVP_aes/i.test(n); });
    console.log("chacha/EVP exports in libcrypto.so (" + hits.length + "):");
    console.log(hits.join("\n"));
    var unisec = Process.findModuleByName("libunisec_x86_64.so");
    if (unisec) {
        var un = unisec.enumerateExports().map(function(e) { return e.name; });
        console.log("libunisec exports sample:");
        console.log(un.slice(0, 40).join("\n"));
    }
}
''')
script.on('message', lambda m, d: print(m.get('payload') if m.get('type') == 'send' else m))
script.load()
session.detach()
