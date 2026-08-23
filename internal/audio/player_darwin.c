#include <AudioToolbox/AudioToolbox.h>
#include <pthread.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

enum {
    MORNLEA_AUDIO_BUFFERS = 8,
    MORNLEA_AUDIO_READY = 0,
    MORNLEA_AUDIO_BUSY = 1,
    MORNLEA_AUDIO_FAILURE = 2,
};

typedef struct {
    AudioQueueBufferRef buffer;
    int busy;
} mornlea_audio_buffer;

typedef struct audio_player {
    AudioQueueRef queue;
    pthread_mutex_t mutex;
    mornlea_audio_buffer buffers[MORNLEA_AUDIO_BUFFERS];
    uint32_t max_samples;
    int started;
} audio_player;

static void mornlea_audio_finished(void *user_data, AudioQueueRef queue, AudioQueueBufferRef buffer) {
    (void)queue;
    audio_player *player = user_data;
    pthread_mutex_lock(&player->mutex);
    for (int index = 0; index < MORNLEA_AUDIO_BUFFERS; index++) {
        if (player->buffers[index].buffer == buffer) {
            player->buffers[index].busy = 0;
            break;
        }
    }
    pthread_mutex_unlock(&player->mutex);
}

audio_player *mornlea_audio_create(float volume, uint32_t max_samples) {
    audio_player *player = calloc(1, sizeof(*player));
    if (player == NULL || max_samples == 0) {
        free(player);
        return NULL;
    }
    if (pthread_mutex_init(&player->mutex, NULL) != 0) {
        free(player);
        return NULL;
    }

    AudioStreamBasicDescription format = {0};
    format.mSampleRate = 22050;
    format.mFormatID = kAudioFormatLinearPCM;
    format.mFormatFlags = kAudioFormatFlagIsSignedInteger | kAudioFormatFlagIsPacked;
    format.mBytesPerPacket = sizeof(int16_t);
    format.mFramesPerPacket = 1;
    format.mBytesPerFrame = sizeof(int16_t);
    format.mChannelsPerFrame = 1;
    format.mBitsPerChannel = 16;
    if (AudioQueueNewOutput(&format, mornlea_audio_finished, player, NULL, NULL, 0, &player->queue) != noErr ||
        AudioQueueSetParameter(player->queue, kAudioQueueParam_Volume, volume) != noErr) {
        if (player->queue != NULL) {
            AudioQueueDispose(player->queue, true);
        }
        pthread_mutex_destroy(&player->mutex);
        free(player);
        return NULL;
    }
    player->max_samples = max_samples;
    for (int index = 0; index < MORNLEA_AUDIO_BUFFERS; index++) {
        if (AudioQueueAllocateBuffer(player->queue, max_samples * sizeof(int16_t), &player->buffers[index].buffer) != noErr) {
            AudioQueueDispose(player->queue, true);
            pthread_mutex_destroy(&player->mutex);
            free(player);
            return NULL;
        }
    }
    return player;
}

int mornlea_audio_play(audio_player *player, const int16_t *pcm, uint32_t samples) {
    if (player == NULL || pcm == NULL || samples == 0 || samples > player->max_samples) {
        return MORNLEA_AUDIO_FAILURE;
    }
    pthread_mutex_lock(&player->mutex);
    mornlea_audio_buffer *slot = NULL;
    for (int index = 0; index < MORNLEA_AUDIO_BUFFERS; index++) {
        if (!player->buffers[index].busy) {
            slot = &player->buffers[index];
            slot->busy = 1;
            break;
        }
    }
    if (slot == NULL) {
        pthread_mutex_unlock(&player->mutex);
        return MORNLEA_AUDIO_BUSY;
    }

    memcpy(slot->buffer->mAudioData, pcm, samples * sizeof(int16_t));
    slot->buffer->mAudioDataByteSize = samples * sizeof(int16_t);
    if (AudioQueueEnqueueBuffer(player->queue, slot->buffer, 0, NULL) != noErr) {
        slot->busy = 0;
        pthread_mutex_unlock(&player->mutex);
        return MORNLEA_AUDIO_FAILURE;
    }
    if (!player->started) {
        if (AudioQueueStart(player->queue, NULL) != noErr) {
            pthread_mutex_unlock(&player->mutex);
            return MORNLEA_AUDIO_FAILURE;
        }
        player->started = 1;
    }
    pthread_mutex_unlock(&player->mutex);
    return MORNLEA_AUDIO_READY;
}

void mornlea_audio_close(audio_player *player) {
    if (player == NULL) {
        return;
    }
    AudioQueueDispose(player->queue, true);
    pthread_mutex_destroy(&player->mutex);
    free(player);
}

audio_player *audio_player_create(float volume, uint32_t max_samples) {
    return mornlea_audio_create(volume, max_samples);
}

int audio_player_play(audio_player *player, const int16_t *pcm, uint32_t samples) {
    return mornlea_audio_play(player, pcm, samples);
}

void audio_player_close(audio_player *player) {
    mornlea_audio_close(player);
}
