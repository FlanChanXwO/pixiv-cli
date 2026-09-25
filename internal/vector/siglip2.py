"""Private stdio bridge for the pinned local SigLIP2 image encoder."""
import json
import sys

try:
    import torch
    import transformers
    from PIL import Image, __version__ as pillow_version
    from transformers import AutoModel, AutoProcessor
except ImportError as exc:
    print(json.dumps({"startup_error": "dependency_missing", "detail": exc.name or "unknown"}), flush=True)
    sys.exit(0)

WANT = ("2.14.0", "5.17.0", "12.1.1")
GOT = (torch.__version__.split("+")[0], transformers.__version__, pillow_version)
if GOT != WANT:
    print(json.dumps({"startup_error": "dependency_version_mismatch", "detail": "torch=%s transformers=%s pillow=%s, want torch=%s transformers=%s pillow=%s" % (GOT + WANT)}), flush=True)
    sys.exit(0)

model_id, revision = sys.argv[1:3]
try:
    processor = AutoProcessor.from_pretrained(model_id, revision=revision, local_files_only=True, trust_remote_code=False)
    model = AutoModel.from_pretrained(model_id, revision=revision, local_files_only=True, use_safetensors=True, trust_remote_code=False).eval()
except OSError:
    print(json.dumps({"startup_error": "model_not_found", "detail": "preload %s@%s into the local transformers cache" % (model_id, revision)}), flush=True)
    sys.exit(0)
except Exception as exc:  # noqa: BLE001 - startup failures must stay diagnosable, not fatal-looking
    print(json.dumps({"startup_error": "model_load_failed", "detail": type(exc).__name__}), flush=True)
    sys.exit(0)

print(json.dumps({"ready": True}), flush=True)
for line in sys.stdin:
    request = json.loads(line)
    try:
        with torch.inference_mode():
            if "text" in request:
                features = model.get_text_features(**processor(text=[request["text"]], padding="max_length", return_tensors="pt")).pooler_output
            else:
                with Image.open(request["image"]) as source:
                    image = source.convert("RGB")
                features = model.get_image_features(**processor(images=[image], return_tensors="pt")).pooler_output
            vector = torch.nn.functional.normalize(features.float(), dim=-1)[0].tolist()
        print(json.dumps({"vector": vector}), flush=True)
    except (OSError, ValueError, KeyError):
        print(json.dumps({"error": "cannot_encode_text" if "text" in request else "cannot_read_image"}), flush=True)
