package vectormodel

const gpuVersion = "0.6.0"

var gpuPlatforms = map[string]nativeSpec{
	"darwin/arm64": {platform: "osx-arm64", library: "libonnxruntime_providers_webgpu.dylib", assets: []asset{
		{path: "libonnxruntime_providers_webgpu.dylib", url: "https://api.nuget.org/v3-flatcontainer/microsoft.ml.onnxruntime.ep.webgpu/0.6.0/microsoft.ml.onnxruntime.ep.webgpu.0.6.0.nupkg#runtimes/osx-arm64/native/libonnxruntime_providers_webgpu.dylib", digest: "5a52ebaa356ccee1e3ce115793466839571ea906b4b37cbc68aef579f4464792", size: 15024432},
	}},
	"linux/amd64": {platform: "linux-x64", library: "libonnxruntime_providers_webgpu.so", assets: []asset{
		{path: "libonnxruntime_providers_webgpu.so", url: "https://api.nuget.org/v3-flatcontainer/microsoft.ml.onnxruntime.ep.webgpu/0.6.0/microsoft.ml.onnxruntime.ep.webgpu.0.6.0.nupkg#runtimes/linux-x64/native/libonnxruntime_providers_webgpu.so", digest: "5b711b9dcececc2d08ffb33d3847df4c7d24804c2d1ff93aa26d2d11891c9916", size: 17387560},
	}},
	"linux/arm64": {platform: "linux-arm64", library: "libonnxruntime_providers_webgpu.so", assets: []asset{
		{path: "libonnxruntime_providers_webgpu.so", url: "https://api.nuget.org/v3-flatcontainer/microsoft.ml.onnxruntime.ep.webgpu/0.6.0/microsoft.ml.onnxruntime.ep.webgpu.0.6.0.nupkg#runtimes/linux-arm64/native/libonnxruntime_providers_webgpu.so", digest: "f883094e682f385177cc1ff536005166d18dea8647daeb3d4ae0d030f0fce659", size: 14778272},
	}},
	"windows/amd64": {platform: "win-x64", library: "onnxruntime_providers_webgpu.dll", assets: []asset{
		{path: "D3D12/D3D12Core.dll", url: "https://api.nuget.org/v3-flatcontainer/microsoft.ml.onnxruntime.ep.webgpu/0.6.0/microsoft.ml.onnxruntime.ep.webgpu.0.6.0.nupkg#runtimes/win-x64/native/D3D12/D3D12Core.dll", digest: "97e081df6724064c1d1a6cc9de0e9cf5817ada128eca6e0e8201e79be15a4f9a", size: 5814072},
		{path: "dxcompiler.dll", url: "https://api.nuget.org/v3-flatcontainer/microsoft.ml.onnxruntime.ep.webgpu/0.6.0/microsoft.ml.onnxruntime.ep.webgpu.0.6.0.nupkg#runtimes/win-x64/native/dxcompiler.dll", digest: "d833eb71611ae86bcfe73092f8daf5d89c1c084f3b31719b53f9884da64842ba", size: 17986360},
		{path: "onnxruntime_providers_webgpu.dll", url: "https://api.nuget.org/v3-flatcontainer/microsoft.ml.onnxruntime.ep.webgpu/0.6.0/microsoft.ml.onnxruntime.ep.webgpu.0.6.0.nupkg#runtimes/win-x64/native/onnxruntime_providers_webgpu.dll", digest: "6bf102c322d6bad5b1b5a04b0b0c3d43191f70f428ad1d6b4c43719674fc57d9", size: 14026040},
	}},
	"windows/arm64": {platform: "win-arm64", library: "onnxruntime_providers_webgpu.dll", assets: []asset{
		{path: "D3D12/D3D12Core.dll", url: "https://api.nuget.org/v3-flatcontainer/microsoft.ml.onnxruntime.ep.webgpu/0.6.0/microsoft.ml.onnxruntime.ep.webgpu.0.6.0.nupkg#runtimes/win-arm64/native/D3D12/D3D12Core.dll", digest: "5c3d1b2e7157d6376f7b72ebf766216b2a52a4710253a78486a7274de7587cb1", size: 5968696},
		{path: "dxcompiler.dll", url: "https://api.nuget.org/v3-flatcontainer/microsoft.ml.onnxruntime.ep.webgpu/0.6.0/microsoft.ml.onnxruntime.ep.webgpu.0.6.0.nupkg#runtimes/win-arm64/native/dxcompiler.dll", digest: "a4336c4ff2fa4990f262a055df76ca1b7d13912dced60134565aa94d75b585b3", size: 22285624},
		{path: "onnxruntime_providers_webgpu.dll", url: "https://api.nuget.org/v3-flatcontainer/microsoft.ml.onnxruntime.ep.webgpu/0.6.0/microsoft.ml.onnxruntime.ep.webgpu.0.6.0.nupkg#runtimes/win-arm64/native/onnxruntime_providers_webgpu.dll", digest: "1a8d55c7141c6588472ccd073c27fb238ce9934954da4238979c1542adf6a94d", size: 14544224},
	}},
}
