#include <unicorn/unicorn.h>
#include <stdio.h>
int main(void) {
    if (!uc_arch_supported(UC_ARCH_ARM64)) { fputs("ARM64 Store runtime is not enabled\n",stderr); return 1; }
    puts("ARM64 Store runtime architecture verified"); return 0;
}
