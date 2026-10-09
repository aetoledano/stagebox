#ifndef STAGEBOX_AUDIO_H
#define STAGEBOX_AUDIO_H

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct { char *id; char *name; int current; } sb_audio_device;
typedef struct { char *id; char *name; int available; int volume; int muted; } sb_audio_status;

uint32_t sb_audio_list(sb_audio_device **devices, unsigned int *count);
void sb_audio_free_devices(sb_audio_device *devices, unsigned int count);
uint32_t sb_audio_volume(int operation, int value, sb_audio_status *status);
void sb_audio_free_status(sb_audio_status *status);
uint32_t sb_audio_set_output(const char *id);

#ifdef __cplusplus
}
#endif
#endif
