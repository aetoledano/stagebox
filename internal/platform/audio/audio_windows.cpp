//go:build windows && cgo

#include "audio_windows.h"
#include <windows.h>
#include <mmdeviceapi.h>
#include <endpointvolume.h>
// Emit property-key definitions here; Ubuntu's MinGW uuid library omits them.
#include <initguid.h>
#include <functiondiscoverykeys_devpkey.h>
#include <cstdlib>
#include <cmath>
#include <algorithm>

template<class T> struct ComPtr {
    T *ptr = nullptr;
    ~ComPtr() { if (ptr) ptr->Release(); }
    T *operator->() const { return ptr; }
};

struct Apartment {
    HRESULT hr = CoInitializeEx(nullptr, COINIT_MULTITHREADED);
    ~Apartment() { if (SUCCEEDED(hr)) CoUninitialize(); }
    HRESULT result() const { return hr == RPC_E_CHANGED_MODE ? S_OK : hr; }
};

static HRESULT enumerator(IMMDeviceEnumerator **out) {
    return CoCreateInstance(__uuidof(MMDeviceEnumerator), nullptr, CLSCTX_INPROC_SERVER,
                            __uuidof(IMMDeviceEnumerator), reinterpret_cast<void **>(out));
}

static char *utf8(const wchar_t *text) {
    int size = WideCharToMultiByte(CP_UTF8, 0, text, -1, nullptr, 0, nullptr, nullptr);
    if (!size) return nullptr;
    char *out = static_cast<char *>(std::malloc(size));
    if (out && !WideCharToMultiByte(CP_UTF8, 0, text, -1, out, size, nullptr, nullptr)) {
        std::free(out);
        return nullptr;
    }
    return out;
}

static HRESULT describe(IMMDevice *device, char **id, char **name) {
    LPWSTR nativeID = nullptr;
    HRESULT hr = device->GetId(&nativeID);
    if (FAILED(hr)) return hr;
    *id = utf8(nativeID);
    CoTaskMemFree(nativeID);
    if (!*id) return E_OUTOFMEMORY;
    ComPtr<IPropertyStore> props;
    hr = device->OpenPropertyStore(STGM_READ, &props.ptr);
    if (FAILED(hr)) return hr;
    PROPVARIANT value;
    PropVariantInit(&value);
    hr = props->GetValue(PKEY_Device_FriendlyName, &value);
    if (SUCCEEDED(hr) && value.vt == VT_LPWSTR && value.pwszVal) *name = utf8(value.pwszVal);
    PropVariantClear(&value);
    if (FAILED(hr)) return hr;
    return *name ? S_OK : E_FAIL;
}

extern "C" void sb_audio_free_devices(sb_audio_device *devices, unsigned int count) {
    for (unsigned int i = 0; i < count; ++i) {
        std::free(devices[i].id);
        std::free(devices[i].name);
    }
    std::free(devices);
}

extern "C" uint32_t sb_audio_list(sb_audio_device **out, unsigned int *count) {
    *out = nullptr;
    *count = 0;
    Apartment apartment;
    if (FAILED(apartment.result())) return apartment.result();
    ComPtr<IMMDeviceEnumerator> devices;
    HRESULT hr = enumerator(&devices.ptr);
    if (FAILED(hr)) return hr;
    ComPtr<IMMDeviceCollection> collection;
    hr = devices->EnumAudioEndpoints(eRender, DEVICE_STATE_ACTIVE, &collection.ptr);
    if (FAILED(hr)) return hr;
    UINT total = 0;
    hr = collection->GetCount(&total);
    if (FAILED(hr) || !total) return hr;
    ComPtr<IMMDevice> current;
    LPWSTR currentID = nullptr;
    hr = devices->GetDefaultAudioEndpoint(eRender, eMultimedia, &current.ptr);
    if (SUCCEEDED(hr)) hr = current->GetId(&currentID);
    if (FAILED(hr) && hr != HRESULT_FROM_WIN32(ERROR_NOT_FOUND)) return hr;
    auto items = static_cast<sb_audio_device *>(std::calloc(total, sizeof(sb_audio_device)));
    if (!items) { CoTaskMemFree(currentID); return E_OUTOFMEMORY; }
    for (UINT i = 0; i < total; ++i) {
        ComPtr<IMMDevice> device;
        hr = collection->Item(i, &device.ptr);
        if (SUCCEEDED(hr)) hr = describe(device.ptr, &items[i].id, &items[i].name);
        if (FAILED(hr)) {
            sb_audio_free_devices(items, total);
            CoTaskMemFree(currentID);
            return hr;
        }
        char *defaultID = currentID ? utf8(currentID) : nullptr;
        items[i].current = defaultID && _stricmp(defaultID, items[i].id) == 0;
        std::free(defaultID);
    }
    CoTaskMemFree(currentID);
    *out = items;
    *count = total;
    return S_OK;
}

extern "C" void sb_audio_free_status(sb_audio_status *status) {
    std::free(status->id);
    std::free(status->name);
}

extern "C" uint32_t sb_audio_volume(int operation, int value, sb_audio_status *status) {
    *status = {};
    Apartment apartment;
    if (FAILED(apartment.result())) return apartment.result();
    ComPtr<IMMDeviceEnumerator> devices;
    HRESULT hr = enumerator(&devices.ptr);
    if (FAILED(hr)) return hr;
    ComPtr<IMMDevice> device;
    hr = devices->GetDefaultAudioEndpoint(eRender, eMultimedia, &device.ptr);
    if (hr == HRESULT_FROM_WIN32(ERROR_NOT_FOUND) && operation == 0) return S_OK;
    if (FAILED(hr)) return hr;
    ComPtr<IAudioEndpointVolume> volume;
    hr = device->Activate(__uuidof(IAudioEndpointVolume), CLSCTX_INPROC_SERVER, nullptr,
                          reinterpret_cast<void **>(&volume.ptr));
    if (FAILED(hr)) return hr;
    float level = 0;
    BOOL muted = FALSE;
    hr = volume->GetMasterVolumeLevelScalar(&level);
    if (SUCCEEDED(hr)) hr = volume->GetMute(&muted);
    if (FAILED(hr)) return hr;
    if (operation == 1 || operation == 2) {
        float next = operation == 1 ? level + value / 100.0f : value / 100.0f;
        hr = volume->SetMasterVolumeLevelScalar(std::max(0.0f, std::min(1.0f, next)), nullptr);
        if (SUCCEEDED(hr)) hr = volume->SetMute(FALSE, nullptr);
    } else if (operation == 3) {
        hr = volume->SetMute(!muted, nullptr);
    }
    if (SUCCEEDED(hr)) hr = volume->GetMasterVolumeLevelScalar(&level);
    if (SUCCEEDED(hr)) hr = volume->GetMute(&muted);
    if (SUCCEEDED(hr)) hr = describe(device.ptr, &status->id, &status->name);
    if (FAILED(hr)) return hr;
    status->available = 1;
    status->volume = static_cast<int>(std::lround(level * 100.0f));
    status->muted = muted;
    return S_OK;
}

extern "C" uint32_t sb_audio_set_output(const char *id) {
    Apartment apartment;
    if (FAILED(apartment.result())) return apartment.result();
    int length = MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, id, -1, nullptr, 0);
    if (!length) return E_INVALIDARG;
    auto target = static_cast<wchar_t *>(std::calloc(length, sizeof(wchar_t)));
    if (!target) return E_OUTOFMEMORY;
    MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, id, -1, target, length);
    ComPtr<IMMDeviceEnumerator> devices;
    HRESULT hr = enumerator(&devices.ptr);
    ComPtr<IMMDevice> device;
    if (SUCCEEDED(hr)) hr = devices->GetDevice(target, &device.ptr);
    DWORD state = 0;
    if (SUCCEEDED(hr)) hr = device->GetState(&state);
    if (SUCCEEDED(hr) && state != DEVICE_STATE_ACTIVE) hr = HRESULT_FROM_WIN32(ERROR_DEVICE_NOT_CONNECTED);
    ComPtr<IMMEndpoint> endpoint;
    if (SUCCEEDED(hr)) hr = device->QueryInterface(__uuidof(IMMEndpoint), reinterpret_cast<void **>(&endpoint.ptr));
    EDataFlow flow = eAll;
    if (SUCCEEDED(hr)) hr = endpoint->GetDataFlow(&flow);
    if (SUCCEEDED(hr) && flow != eRender) hr = E_INVALIDARG;
    if (FAILED(hr)) { std::free(target); return hr; }

    // Windows exposes default-output switching through this private interface.
    // Layout reference: github.com/tartakynov/audioswitch/blob/master/IPolicyConfig.h
    const GUID clsid = {0x870af99c,0x171d,0x4f9e,{0xaf,0x0d,0xe6,0x3d,0xf4,0x0c,0x2b,0xc9}};
    const GUID iid = {0xf8679f50,0x850a,0x41cf,{0x9c,0x72,0x43,0x0f,0x29,0x02,0x90,0xc8}};
    ComPtr<IUnknown> policy;
    hr = CoCreateInstance(clsid, nullptr, CLSCTX_INPROC_SERVER, iid, reinterpret_cast<void **>(&policy.ptr));
    if (FAILED(hr)) { std::free(target); return hr; }
    using SetDefault = HRESULT (STDMETHODCALLTYPE *)(IUnknown *, LPCWSTR, ERole);
    auto setDefault = reinterpret_cast<SetDefault>((*reinterpret_cast<void ***>(policy.ptr))[13]);
    LPWSTR oldIDs[3] = {};
    for (int role = 0; role < 3; ++role) {
        ComPtr<IMMDevice> old;
        if (SUCCEEDED(devices->GetDefaultAudioEndpoint(eRender, static_cast<ERole>(role), &old.ptr)))
            old->GetId(&oldIDs[role]);
    }
    for (int role = 0; role < 3; ++role) {
        hr = setDefault(policy.ptr, target, static_cast<ERole>(role));
        if (FAILED(hr)) {
            for (int previous = 0; previous < role; ++previous)
                if (oldIDs[previous]) setDefault(policy.ptr, oldIDs[previous], static_cast<ERole>(previous));
            break;
        }
    }
    for (auto oldID : oldIDs) CoTaskMemFree(oldID);
    std::free(target);
    return hr;
}
