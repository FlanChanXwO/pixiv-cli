"""Private stdio bridge for the pinned local SigLIP2 image encoder."""
import json
import sys

import torch
import transformers
from PIL import Image, __version__ as pillow_version
from transformers import AutoModel, AutoProcessor

if (torch.__version__.split("+")[0], transformers.__version__, pillow_version) != ("2.14.0", "5.17.0", "12.1.1"):
    sys.exit(1)

model_id, revision = sys.argv[1:3]
processor = AutoProcessor.from_pretrained(model_id, revision=revision, local_files_only=True, trust_remote_code=False)
model = AutoModel.from_pretrained(model_id, revision=revision, local_files_only=True, use_safetensors=True, trust_remote_code=False).eval()
print(json.dumps({"ready": True}), flush=True)
for line in sys.stdin:
    request = json.loads(line)
    try:
        with Image.open(request["image"]) as source:
            image = source.convert("RGB")
        with torch.inference_mode():
            features = model.get_image_features(**processor(images=[image], return_tensors="pt")).pooler_output
            vector = torch.nn.functional.normalize(features.float(), dim=-1)[0].tolist()
        print(json.dumps({"vector": vector}), flush=True)
    except (OSError, ValueError, KeyError):
        print(json.dumps({"error": "cannot_read_image"}), flush=True)
