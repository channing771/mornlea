#include <AudioToolbox/AudioToolbox.h>
#include <stdint.h>
#include <stdatomic.h>
#include <stdlib.h>
#include <string.h>

enum {
    MORNLEA_AUDIO_BUFFERS = 8,
    MORNLEA_AUDIO_READY = 0,
    MORNLEA_AUDIO_BUSY = 1,
    MORNLEA_AUDIO_FAILURE = 2,
    MORNLEA_AUDIO_TEST_MAX_SAMPLES = 3087,
};

typedef struct {
    AudioQueueBufferRef buffer;
} mornlea_audio_buffer;

typedef struct audio_player {
    AudioQueueRef queue;
    mornlea_audio_buffer buffers[MORNLEA_AUDIO_BUFFERS];
    atomic_int busy[MORNLEA_AUDIO_BUFFERS];
    uint32_t max_samples;
    int started;
} audio_player;

typedef struct audio_player_test_state {
    atomic_int busy[MORNLEA_AUDIO_BUFFERS];
    int16_t pcm[MORNLEA_AUDIO_BUFFERS][MORNLEA_AUDIO_TEST_MAX_SAMPLES];
    int last_slot;
    int fail_next;
    int failed;
} audio_player_test_state;

// 回调线程只通过 release store 归还 buffer；客户确认线程的 acquire CAS 认领它。
static int mornlea_audio_claim_slot(atomic_int busy[MORNLEA_AUDIO_BUFFERS]) {
    for (int index = 0; index < MORNLEA_AUDIO_BUFFERS; index++) {
        int available = 0;
        if (atomic_compare_exchange_strong_explicit(
                &busy[index], &available, 1, memory_order_acquire, memory_order_relaxed)) {
            return index;
        }
    }
    return -1;
}

static void mornlea_audio_release_slot(atomic_int busy[MORNLEA_AUDIO_BUFFERS], int index) {
    atomic_store_explicit(&busy[index], 0, memory_order_release);
}

static void mornlea_audio_finished(void *user_data, AudioQueueRef queue, AudioQueueBufferRef buffer) {
    (void)queue;
    audio_player *player = user_data;
    for (int index = 0; index < MORNLEA_AUDIO_BUFFERS; index++) {
        if (player->buffers[index].buffer == buffer) {
            mornlea_audio_release_slot(player->busy, index);
            break;
        }
    }
}

audio_player *mornlea_audio_create(float volume, uint32_t max_samples) {
    audio_player *player = calloc(1, sizeof(*player));
    if (player == NULL || max_samples == 0) {
        free(player);
        return NULL;
    }
    for (int index = 0; index < MORNLEA_AUDIO_BUFFERS; index++) {
        atomic_init(&player->busy[index], 0);
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
        free(player);
        return NULL;
    }
    player->max_samples = max_samples;
    for (int index = 0; index < MORNLEA_AUDIO_BUFFERS; index++) {
        if (AudioQueueAllocateBuffer(player->queue, max_samples * sizeof(int16_t), &player->buffers[index].buffer) != noErr) {
            AudioQueueDispose(player->queue, true);
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
    int index = mornlea_audio_claim_slot(player->busy);
    if (index < 0) {
        return MORNLEA_AUDIO_BUSY;
    }
    mornlea_audio_buffer *slot = &player->buffers[index];

    memcpy(slot->buffer->mAudioData, pcm, samples * sizeof(int16_t));
    slot->buffer->mAudioDataByteSize = samples * sizeof(int16_t);
    if (AudioQueueEnqueueBuffer(player->queue, slot->buffer, 0, NULL) != noErr) {
        mornlea_audio_release_slot(player->busy, index);
        return MORNLEA_AUDIO_FAILURE;
    }
    if (!player->started) {
        if (AudioQueueStart(player->queue, NULL) != noErr) {
            return MORNLEA_AUDIO_FAILURE;
        }
        player->started = 1;
    }
    return MORNLEA_AUDIO_READY;
}

void mornlea_audio_close(audio_player *player) {
    if (player == NULL) {
        return;
    }
    AudioQueueDispose(player->queue, true);
    free(player);
}

audio_player_test_state *audio_player_test_create(void) {
    audio_player_test_state *state = calloc(1, sizeof(*state));
    if (state == NULL) {
        return NULL;
    }
    for (int index = 0; index < MORNLEA_AUDIO_BUFFERS; index++) {
        atomic_init(&state->busy[index], 0);
    }
    state->last_slot = -1;
    return state;
}

void audio_player_test_close(audio_player_test_state *state) {
    free(state);
}

int audio_player_test_play(audio_player_test_state *state, const int16_t *pcm, uint32_t samples) {
    if (state == NULL || pcm == NULL || samples == 0 || samples > MORNLEA_AUDIO_TEST_MAX_SAMPLES || state->failed) {
        return MORNLEA_AUDIO_FAILURE;
    }
    int index = mornlea_audio_claim_slot(state->busy);
    if (index < 0) {
        return MORNLEA_AUDIO_BUSY;
    }
    memcpy(state->pcm[index], pcm, samples * sizeof(int16_t));
    state->last_slot = index;
    if (state->fail_next) {
        state->fail_next = 0;
        mornlea_audio_release_slot(state->busy, index);
        state->failed = 1;
        return MORNLEA_AUDIO_FAILURE;
    }
    return MORNLEA_AUDIO_READY;
}

int audio_player_test_last_slot(audio_player_test_state *state) {
    return state == NULL ? -1 : state->last_slot;
}

int audio_player_test_busy_count(audio_player_test_state *state) {
    if (state == NULL) {
        return 0;
    }
    int count = 0;
    for (int index = 0; index < MORNLEA_AUDIO_BUFFERS; index++) {
        count += atomic_load_explicit(&state->busy[index], memory_order_acquire) != 0;
    }
    return count;
}

void audio_player_test_finish(audio_player_test_state *state, int slot) {
    if (state != NULL && slot >= 0 && slot < MORNLEA_AUDIO_BUFFERS) {
        mornlea_audio_release_slot(state->busy, slot);
    }
}

void audio_player_test_fail_next(audio_player_test_state *state) {
    if (state != NULL) {
        state->fail_next = 1;
    }
}

int audio_player_test_failed(audio_player_test_state *state) {
    return state != NULL && state->failed;
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
