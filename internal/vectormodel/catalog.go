package vectormodel

const (
	// DefaultModel 是本地 setup 的默认模型。
	DefaultModel    = "siglip2-base-512"
	runtimeVersion  = "1.32.0"
	localDimensions = 768
	textLength      = 64
	textEOS         = 1
)

type asset struct {
	path, url, digest string
	size              int64
}

type modelSpec struct {
	name, revision string
	size           int
	assets         []asset
}

var localModels = map[string]modelSpec{
	"siglip2-base-512": {name: "siglip2-base-512", revision: "323fe16b35d8ced8182f8d9ff5e5ec935dd79bd7", size: 512, assets: []asset{
		{path: "tokenizer.model", url: "https://huggingface.co/onnx-community/siglip2-base-patch16-512-ONNX/resolve/323fe16b35d8ced8182f8d9ff5e5ec935dd79bd7/tokenizer.model", digest: "61a7b147390c64585d6c3543dd6fc636906c9af3865a5548f27f31aee1d4c8e2", size: 4241003},
		{path: "onnx/vision_model.onnx", url: "https://huggingface.co/onnx-community/siglip2-base-patch16-512-ONNX/resolve/323fe16b35d8ced8182f8d9ff5e5ec935dd79bd7/onnx/vision_model.onnx", digest: "a2c02e842727cc1c5661e54d4e7671676781c4efc0047d3d17f6211593e12b4e", size: 374352089},
		{path: "onnx/text_model.onnx", url: "https://huggingface.co/onnx-community/siglip2-base-patch16-512-ONNX/resolve/323fe16b35d8ced8182f8d9ff5e5ec935dd79bd7/onnx/text_model.onnx", digest: "469679f9160d3ddfe72904c8b91d2ddeab3fa08dda727877c34ff16d77b9e31a", size: 1129469657},
	}},
	"siglip2-base-256": {name: "siglip2-base-256", revision: "d1114256522a37ffa257a0a58017348ab0058db2", size: 256, assets: []asset{
		{path: "tokenizer.model", url: "https://huggingface.co/onnx-community/siglip2-base-patch16-256-ONNX/resolve/d1114256522a37ffa257a0a58017348ab0058db2/tokenizer.model", digest: "61a7b147390c64585d6c3543dd6fc636906c9af3865a5548f27f31aee1d4c8e2", size: 4241003},
		{path: "onnx/vision_model.onnx", url: "https://huggingface.co/onnx-community/siglip2-base-patch16-256-ONNX/resolve/d1114256522a37ffa257a0a58017348ab0058db2/onnx/vision_model.onnx", digest: "f5cb16728a704703f05516ded628397e11dbca4de2eb5db04b0c0bcee988aa7a", size: 371992072},
		{path: "onnx/text_model.onnx", url: "https://huggingface.co/onnx-community/siglip2-base-patch16-256-ONNX/resolve/d1114256522a37ffa257a0a58017348ab0058db2/onnx/text_model.onnx", digest: "d3de4a6bbbfcb429b6615ac496790353cf4a4fc0f19fbbe7179e523ae60daaef", size: 1129469657},
	}},
}

type nativeSpec struct {
	platform, library string
	assets            []asset
}

var nativePlatforms = map[string]nativeSpec{
	"darwin/arm64": {platform: "osx-arm64", library: "libonnxruntime.dylib", assets: []asset{
		{path: "libonnxruntime.dylib", url: "https://api.nuget.org/v3-flatcontainer/microsoft.ml.onnxruntime/1.32.0/microsoft.ml.onnxruntime.1.32.0.nupkg#runtimes/osx-arm64/native/libonnxruntime.dylib", digest: "21ff304871400e846a30c8a1165d15173add56598afb45f98227e2dfc716a497", size: 47622768},
	}},
	"linux/amd64": {platform: "linux-x64", library: "libonnxruntime.so", assets: []asset{
		{path: "libonnxruntime.so", url: "https://api.nuget.org/v3-flatcontainer/microsoft.ml.onnxruntime/1.32.0/microsoft.ml.onnxruntime.1.32.0.nupkg#runtimes/linux-x64/native/libonnxruntime.so", digest: "4213d6d8c42ac4754865849cb139d31fc39127db6a326babf90bbc81a6eb52fc", size: 34824680},
		{path: "libonnxruntime_providers_shared.so", url: "https://api.nuget.org/v3-flatcontainer/microsoft.ml.onnxruntime/1.32.0/microsoft.ml.onnxruntime.1.32.0.nupkg#runtimes/linux-x64/native/libonnxruntime_providers_shared.so", digest: "8ef51f6bf2c621fc129f625b22241fa6f9a5870bc08eaf2fdd279169ec40d09a", size: 14632},
	}},
	"linux/arm64": {platform: "linux-arm64", library: "libonnxruntime.so", assets: []asset{
		{path: "libonnxruntime.so", url: "https://api.nuget.org/v3-flatcontainer/microsoft.ml.onnxruntime/1.32.0/microsoft.ml.onnxruntime.1.32.0.nupkg#runtimes/linux-arm64/native/libonnxruntime.so", digest: "fd1db07c49cb1ad11370ef4393a405eef959fa9622fb5b2880f85d025757e719", size: 27165936},
		{path: "libonnxruntime_providers_shared.so", url: "https://api.nuget.org/v3-flatcontainer/microsoft.ml.onnxruntime/1.32.0/microsoft.ml.onnxruntime.1.32.0.nupkg#runtimes/linux-arm64/native/libonnxruntime_providers_shared.so", digest: "f91fa2b0a2b8dba9cbc319254d342fa8201fd94b5912b9ce717f82e2207ee3d5", size: 198792},
	}},
	"windows/amd64": {platform: "win-x64", library: "onnxruntime.dll", assets: []asset{
		{path: "onnxruntime.dll", url: "https://api.nuget.org/v3-flatcontainer/microsoft.ml.onnxruntime/1.32.0/microsoft.ml.onnxruntime.1.32.0.nupkg#runtimes/win-x64/native/onnxruntime.dll", digest: "348c3d29d6995eee37e3451825eaddb7ea7400be3afda6ca2286b12bc775f042", size: 20602168},
		{path: "onnxruntime_providers_shared.dll", url: "https://api.nuget.org/v3-flatcontainer/microsoft.ml.onnxruntime/1.32.0/microsoft.ml.onnxruntime.1.32.0.nupkg#runtimes/win-x64/native/onnxruntime_providers_shared.dll", digest: "6caf57ac07c9cf72fb4e73c5bb0fe156f02dd60bbbd7385530c9a5654f10f950", size: 21816},
	}},
	"windows/arm64": {platform: "win-arm64", library: "onnxruntime.dll", assets: []asset{
		{path: "onnxruntime.dll", url: "https://api.nuget.org/v3-flatcontainer/microsoft.ml.onnxruntime/1.32.0/microsoft.ml.onnxruntime.1.32.0.nupkg#runtimes/win-arm64/native/onnxruntime.dll", digest: "d9db6113549a3bd1eae55dbcb96bd95ee1f35c2823f39a42972664abab6517d2", size: 19053920},
		{path: "onnxruntime_providers_shared.dll", url: "https://api.nuget.org/v3-flatcontainer/microsoft.ml.onnxruntime/1.32.0/microsoft.ml.onnxruntime.1.32.0.nupkg#runtimes/win-arm64/native/onnxruntime_providers_shared.dll", digest: "a63690de2f89942e3e3c4dbb9ed77c6b5bfa56def6dc89bff0bdf120df9e5a6d", size: 21344},
	}},
}
