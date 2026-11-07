#include <stdint.h>
int QZSetCacheDirectory(char *path);
uint64_t QZCreate(void);
char *QZCall(uint64_t handle, char *input);
char *QZPoll(uint64_t handle);
void QZCancel(uint64_t handle);
void QZClose(uint64_t handle);
void QZFree(char *pointer);
