package jellycompat

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

const (
	maxDeviceProfileRequestBytes = 1 << 20
	maxDeviceProfileBytes        = 256 << 10
	maxDeviceProfileEntries      = 1024
	maxDeviceIDBytes             = 256
	maxDeviceProfilesPerToken    = 64
	deviceProfilePayloadTooLarge = "PayloadTooLarge"
)

func readDeviceProfileRequest(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxDeviceProfileRequestBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxDeviceProfileRequestBytes {
		return nil, &HTTPError{StatusCode: http.StatusRequestEntityTooLarge, Code: deviceProfilePayloadTooLarge, Message: "Device profile request is too large"}
	}
	return body, nil
}

func encodeDeviceProfile(profile DeviceProfile, deviceID string) ([]byte, error) {
	if len(deviceID) > maxDeviceIDBytes {
		return nil, &HTTPError{StatusCode: http.StatusBadRequest, Code: "BadRequest", Message: "DeviceId is too long"}
	}
	entries := len(profile.DirectPlayProfiles) + len(profile.TranscodingProfiles) + len(profile.ContainerProfiles) + len(profile.CodecProfiles) + len(profile.SubtitleProfiles)
	for _, p := range profile.TranscodingProfiles {
		entries += len(p.Conditions)
	}
	for _, p := range profile.ContainerProfiles {
		entries += len(p.Conditions)
	}
	for _, p := range profile.CodecProfiles {
		entries += len(p.Conditions) + len(p.ApplyConditions)
	}
	if entries > maxDeviceProfileEntries {
		return nil, &HTTPError{StatusCode: http.StatusRequestEntityTooLarge, Code: deviceProfilePayloadTooLarge, Message: "Device profile has too many entries"}
	}
	data, err := json.Marshal(profile)
	if err != nil {
		return nil, err
	}
	if len(data) > maxDeviceProfileBytes {
		return nil, &HTTPError{StatusCode: http.StatusRequestEntityTooLarge, Code: deviceProfilePayloadTooLarge, Message: "Device profile is too large"}
	}
	return data, nil
}

func deviceProfileQuotaError() error {
	return &HTTPError{StatusCode: http.StatusTooManyRequests, Code: "TooManyDevices", Message: "Too many device profiles registered for this token"}
}

func writeDeviceProfileRequestError(w http.ResponseWriter, err error, invalidMessage string) {
	if errors.Is(err, errDeviceProfileStore) {
		writeError(w, http.StatusServiceUnavailable, "Unavailable", "Device profile storage unavailable")
	} else if httpErr, ok := errors.AsType[*HTTPError](err); ok {
		writeError(w, httpErr.StatusCode, httpErr.Code, httpErr.Message)
	} else {
		writeError(w, http.StatusBadRequest, "BadRequest", invalidMessage)
	}
}
