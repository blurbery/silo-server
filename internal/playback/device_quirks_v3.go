package playback

import "strings"

const (
	QuirkFireTVAFTKRTHigh10V3         = "android.fire_tv.aftkrt.h264_high10_l52_v1"
	QuirkFireTVAFTKRTEAC3HLSV3        = "android.fire_tv.aftkrt.eac3_7_1_hls_audio_adapt_v1"
	QuirkAndroidMobileEAC3BluetoothV3 = "android.mobile.eac3_bluetooth_hls_audio_adapt_v1"
	QuirkFireTVDV8HDR10PlusV3         = "android.fire_tv.dv8_hdr10plus_sei_v1"
	QuirkFirefoxHEVCOpenGOPV3         = "web.firefox.hevc_open_gop_resume_v1"
	QuirkFirefoxMatroskaAACTimingV3   = "web.firefox.matroska_aac_timestamps_v1"
	QuirkWindowsWebAudioNormalizeV3   = "web.windows.audio_normalization_v1"
)

func high10DecodeOverrideV3(source SourceDescriptorV3, request StartRequestV3) (*AppliedQuirkV3, bool) {
	if !deviceQuirkProtocolAvailableV3(request) || !isAmazonModelV3(request, "AFTKRT") ||
		!strings.EqualFold(source.VideoCodec, "h264") || !isHigh10ProfileV3(source.VideoProfile) ||
		source.BitDepth != 10 || source.VideoLevel <= 0 || source.VideoLevel > 52 ||
		source.Width <= 0 || source.Height <= 0 || source.Width > 1920 || source.Height > 1080 {
		return nil, false
	}
	for _, capability := range request.Capabilities.VideoDecode {
		if !capability.Hardware || !strings.EqualFold(capability.Codec, "h264") ||
			capability.MaxWidth > 0 && source.Width > capability.MaxWidth ||
			capability.MaxHeight > 0 && source.Height > capability.MaxHeight ||
			capability.MaxFrameRate > 0 && source.FrameRate > capability.MaxFrameRate ||
			capability.MaxBitrateKbps > 0 && source.BitrateKbps > capability.MaxBitrateKbps {
			continue
		}
		quirk := AppliedQuirkV3{
			ID:               QuirkFireTVAFTKRTHigh10V3,
			RegistryRevision: DeviceQuirkRegistryRevisionV3,
			Action:           "positive_decode_override",
			Reason:           "Exact AFTKRT evidence supports hardware H.264 High 10 through level 5.2 at 1080p.",
		}
		return &quirk, true
	}
	return nil, false
}

func hlsEAC3AudioCorrectionV3(source SourceDescriptorV3, request StartRequestV3) (*AppliedQuirkV3, bool) {
	if !deviceQuirkProtocolAvailableV3(request) || !strings.EqualFold(source.AudioCodec, "eac3") {
		return nil, false
	}
	if isAndroidMobileBluetoothOutputV3(request) && source.AudioChannels >= 6 {
		quirk := AppliedQuirkV3{
			ID:               QuirkAndroidMobileEAC3BluetoothV3,
			RegistryRevision: DeviceQuirkRegistryRevisionV3,
			Action:           "audio_only_transcode",
			Reason:           "Android mobile cannot reliably decode multichannel E-AC-3 on the observed Bluetooth output route; preserve video and adapt audio to AAC.",
		}
		return &quirk, true
	}
	if !isAmazonModelV3(request, "AFTKRT") || source.AudioChannels != 8 {
		return nil, false
	}
	quirk := AppliedQuirkV3{
		ID:               QuirkFireTVAFTKRTEAC3HLSV3,
		RegistryRevision: DeviceQuirkRegistryRevisionV3,
		Action:           "audio_only_transcode",
		Reason:           "AFTKRT cannot reliably consume eight-channel E-AC-3 from an HLS MPEG-TS route.",
	}
	return &quirk, true
}

func isAndroidMobileBluetoothOutputV3(request StartRequestV3) bool {
	device := request.ClientPlaybackContext.Device
	output := request.ClientPlaybackContext.Output
	return strings.EqualFold(device.Platform, "android") &&
		strings.EqualFold(request.ClientPlaybackContext.FormFactor, "mobile") &&
		strings.EqualFold(output.SinkType, "bluetooth") &&
		output.AudioPassthrough != nil &&
		len(output.AudioPassthrough.PassthroughCodecs) == 0
}

// usesFirstPartyAndroidMedia3HLSV3 identifies Silo Android's native Media3 HLS
// engine. The device-quirks protocol is the first-party opt-in: an unrelated
// client cannot obtain Android-specific byte recipes by sending only the broad
// platform label.
func usesFirstPartyAndroidMedia3HLSV3(request StartRequestV3) bool {
	return deviceQuirkProtocolAvailableV3(request) &&
		strings.EqualFold(strings.TrimSpace(request.ClientPlaybackContext.Device.Platform), "android")
}

func dv8HDR10PlusRuntimeCorrectionV3(source SourceDescriptorV3, request StartRequestV3, deliveryClass string) (*AppliedQuirkV3, bool) {
	if !deviceQuirkProtocolAvailableV3(request) || source.DVProfile != 8 || !source.HDR10Plus ||
		!isAmazonFireTVV3(request) || !deliverySupportsFeatureV3(request, deliveryClass, ClientDV8HDR10PlusSanitizerV3) {
		return nil, false
	}
	quirk := AppliedQuirkV3{
		ID:               QuirkFireTVDV8HDR10PlusV3,
		RegistryRevision: DeviceQuirkRegistryRevisionV3,
		Action:           "client_runtime_correction",
		Reason:           "The native Fire TV Dolby Vision path requires HDR10+ dynamic-metadata SEI removal for hybrid Profile 8 samples.",
	}
	return &quirk, true
}

// firefoxHEVCOpenGOPQuirkV3 marks copied HEVC remux recipes whose resumed
// byte stream needs its initial open-GOP leading pictures normalized. Firefox
// rejects those pre-key pictures after a demuxer seek even though it decodes
// the same MKV from the beginning. The serving layer executes the filter only
// for a non-zero seek, but the plan freezes the quirk from the first start so a
// later seek reanchor cannot lose the byte recipe.
func firefoxHEVCOpenGOPQuirkV3(source SourceDescriptorV3, request StartRequestV3) (*AppliedQuirkV3, bool) {
	if !isFirefoxWebV3(request) || !strings.EqualFold(source.VideoCodec, "hevc") {
		return nil, false
	}
	quirk := AppliedQuirkV3{
		ID:               QuirkFirefoxHEVCOpenGOPV3,
		RegistryRevision: DeviceQuirkRegistryRevisionV3,
		Action:           "server_copy_bitstream_normalization",
		Reason:           "Firefox HEVC resume requires initial open-GOP leading pictures to be removed after a server-side seek.",
	}
	return &quirk, true
}

// firefoxMatroskaAACTimingQuirkV3 prevents millisecond-rounded Matroska AAC
// packet timestamps from being copied into MP4/fMP4. Firefox treats those
// sub-frame gaps as missing audio and inserts silence, which is heard as
// crackling. Other clients keep codec-copy remuxing, and Firefox direct play
// remains available when its native container claim is valid.
func firefoxMatroskaAACTimingQuirkV3(source SourceDescriptorV3, request StartRequestV3) (*AppliedQuirkV3, bool) {
	container := strings.ToLower(strings.TrimSpace(source.Container))
	if !isFirefoxWebV3(request) || (container != containerMKVV3 && container != "matroska") ||
		!strings.EqualFold(strings.TrimSpace(source.AudioCodec), audioCodecAACV3) {
		return nil, false
	}
	quirk := AppliedQuirkV3{
		ID:               QuirkFirefoxMatroskaAACTimingV3,
		RegistryRevision: DeviceQuirkRegistryRevisionV3,
		Action:           "audio_only_transcode",
		Reason:           "Firefox requires Matroska AAC timestamps to be normalized before MP4 or HLS packaging.",
	}
	return &quirk, true
}

// windowsWebAudioNormalizationQuirkV3 keeps video on a source-preserving route
// while giving Windows browsers one stable audio contract. Every non-AAC
// source codec is decoded server-side and re-encoded with the timestamp-
// normalizing AAC recipe. Windows Firefox also normalizes AAC because its
// audio path is the strictest target; other browsers keep native AAC.
func windowsWebAudioNormalizationQuirkV3(source SourceDescriptorV3, request StartRequestV3) (*AppliedQuirkV3, bool) {
	codec := normalizeCodecV3(source.AudioCodec)
	if !isWindowsWebV3(request) || codec == "" || (codec == audioCodecAACV3 && !isFirefoxWebV3(request)) {
		return nil, false
	}
	quirk := AppliedQuirkV3{
		ID:               QuirkWindowsWebAudioNormalizeV3,
		RegistryRevision: DeviceQuirkRegistryRevisionV3,
		Action:           "audio_only_transcode",
		Reason:           "Windows web playback normalizes source audio to timestamp-corrected AAC while preserving copied video; Firefox also normalizes native AAC.",
	}
	return &quirk, true
}

func isWindowsWebV3(request StartRequestV3) bool {
	device := request.ClientPlaybackContext.Device
	if !strings.EqualFold(device.Platform, "web") {
		return false
	}
	userAgent := strings.ToLower(strings.TrimSpace(device.PlatformDetails["user_agent"]))
	return strings.Contains(userAgent, "windows nt")
}

func isFirefoxWebV3(request StartRequestV3) bool {
	device := request.ClientPlaybackContext.Device
	if !strings.EqualFold(device.Platform, "web") {
		return false
	}
	userAgent := strings.ToLower(strings.TrimSpace(device.PlatformDetails["user_agent"]))
	return strings.Contains(userAgent, "firefox/") && !strings.Contains(userAgent, "seamonkey/")
}

func applyFirefoxHEVCOpenGOPQuirkV3(plan *PlanV3, source SourceDescriptorV3, request StartRequestV3) bool {
	quirk, ok := firefoxHEVCOpenGOPQuirkV3(source, request)
	if !ok {
		return false
	}
	appendAppliedQuirkV3(plan, *quirk, "")
	return true
}

func applyCopiedVideoQuirksV3(plan *PlanV3, source SourceDescriptorV3, request StartRequestV3, high10 *AppliedQuirkV3) {
	if high10 != nil {
		appendAppliedQuirkV3(plan, *high10, "")
	}
	if quirk, ok := dv8HDR10PlusRuntimeCorrectionV3(source, request, DeliveryClassV3(plan.Delivery)); ok {
		appendAppliedQuirkV3(plan, *quirk, ClientDV8HDR10PlusSanitizerV3)
	}
}

func appendAppliedQuirkV3(plan *PlanV3, quirk AppliedQuirkV3, runtimeCorrection string) {
	for _, existing := range plan.AppliedQuirks {
		if existing.ID == quirk.ID && existing.RegistryRevision == quirk.RegistryRevision {
			return
		}
	}
	plan.AppliedQuirks = append(plan.AppliedQuirks, quirk)
	if runtimeCorrection != "" && !containsFoldV3(plan.RuntimeCorrections, runtimeCorrection) {
		plan.RuntimeCorrections = append(plan.RuntimeCorrections, runtimeCorrection)
	}
}

func deviceQuirkProtocolAvailableV3(request StartRequestV3) bool {
	return HasFeatureV3(request.ClientFeatures, FeatureDeviceQuirksV3)
}

func deliverySupportsFeatureV3(request StartRequestV3, deliveryClass string, feature string) bool {
	value, ok := request.ClientPlaybackContext.Deliveries[deliveryClass]
	return ok && value.Enabled && value.SupportedOnDevice && HasFeatureV3(value.Features, feature)
}

func isAmazonModelV3(request StartRequestV3, model string) bool {
	device := request.ClientPlaybackContext.Device
	return strings.EqualFold(device.Model, model) && strings.EqualFold(device.Manufacturer, "Amazon")
}

func isAmazonFireTVV3(request StartRequestV3) bool {
	device := request.ClientPlaybackContext.Device
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(device.Model)), "AFT") &&
		strings.EqualFold(device.Manufacturer, "Amazon")
}

func isHigh10ProfileV3(profile string) bool {
	normalized := strings.NewReplacer(" ", "", "-", "", "_", "").Replace(strings.ToLower(profile))
	return normalized == "high10" || normalized == "high10intra"
}
