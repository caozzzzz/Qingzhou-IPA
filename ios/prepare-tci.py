from pathlib import Path
import sys

root = Path(sys.argv[1])
def replace(name, before, after, count=1):
    p = root / name
    text = p.read_text()
    if text.count(before) != count:
        raise SystemExit(f'Unexpected pinned TCI source: {name}')
    p.write_text(text.replace(before, after))

# TCI buffers contain interpreted data, never host instructions. Do not even
# request executable memory or use Apple's JIT thread permissions.
replace('qemu/accel/tcg/translate-all.c',
        'int prot = PROT_WRITE | PROT_READ | PROT_EXEC;',
        '#ifdef CONFIG_TCG_INTERPRETER\n    int prot = PROT_READ | PROT_WRITE;\n#else\n    int prot = PROT_WRITE | PROT_READ | PROT_EXEC;\n#endif')
replace('qemu/accel/tcg/translate-all.c', '#ifdef USE_MAP_JIT',
        '#if defined(USE_MAP_JIT) && !defined(CONFIG_TCG_INTERPRETER)')
replace('qemu/accel/tcg/translate-all.c',
        'if (qemu_mprotect_rwx(buf, size)) {\n        abort();\n    }',
        '#ifndef CONFIG_TCG_INTERPRETER\n    if (qemu_mprotect_rwx(buf, size)) {\n        abort();\n    }\n#endif')
condition = '#if defined(__APPLE__) && defined(HAVE_PTHREAD_JIT_PROTECT)'
replace('qemu/include/tcg/tcg-apple-jit.h', condition,
        '#if !defined(CONFIG_TCG_INTERPRETER) && defined(__APPLE__) && defined(HAVE_PTHREAD_JIT_PROTECT)', count=2)
replace('uc.c', condition,
        '#if !defined(CONFIG_TCG_INTERPRETER) && defined(__APPLE__) && defined(HAVE_PTHREAD_JIT_PROTECT)')
print('Pinned interpreter configured to use read/write data buffers only.')
