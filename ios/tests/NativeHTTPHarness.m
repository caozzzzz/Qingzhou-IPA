#include <stdio.h>
#include <stdlib.h>
char *QZNativeHTTP(const char *requestJSON);
int main(void) {
    char input[1048576];
    if (!fgets(input, sizeof(input), stdin)) return 2;
    char *result=QZNativeHTTP(input);
    if (!result) return 3;
    puts(result); free(result); return 0;
}
