"""Builds internal/aruba/hwmodels.json from the NetBox device type library
(github.com/netbox-community/devicetype-library, CC0): for each Aruba switch
part number, its model name, which front ports are SFP cages, and the name of
its front photo in the library's elevation-images/HPE folder.

  git clone --depth 1 https://github.com/netbox-community/devicetype-library dtl
  python3 tools/netbox/gen.py dtl
"""
import json, os, re, sys
import yaml

lib = sys.argv[1]
imgs = {os.path.basename(p) for p in os.listdir(os.path.join(lib, "elevation-images", "HPE"))}
out = {}
for f in sorted(os.listdir(os.path.join(lib, "device-types", "HPE"))):
    if not f.lower().startswith("aruba") or not f.endswith((".yaml", ".yml")):
        continue
    d = yaml.safe_load(open(os.path.join(lib, "device-types", "HPE", f)))
    pn = re.match(r"(J\d{4}[A-Z]|JL\d{3}[A-Z]|R\d[A-Z0-9]{2}\d{2}[A-Z]|S\d[A-Z0-9]{2}\d{2}[A-Z])", str(d.get("part_number") or "").upper())
    if not pn:
        continue
    sfp = [str(i["name"]) for i in d.get("interfaces") or []
           if not i.get("mgmt_only") and re.search(r"sfp|qsfp|-x-|-x$|xfp", i.get("type", ""))]
    photo = next((n for n in (f'{d["slug"]}.front.png', f'{d["slug"]}.front.jpg') if n in imgs), "")
    # ports are matched by their last number, so 49, 1/49 and 1/1/49 are one port
    last = sorted({n.split("/")[-1] for n in sfp}, key=lambda x: (len(x), x))
    out[pn.group(1)] = {"model": d["model"], "sfp": last, "photo": photo}
dst = os.path.join(os.path.dirname(__file__), "..", "..", "internal", "aruba", "hwmodels.json")
json.dump(out, open(dst, "w"), indent=0, sort_keys=True)
print(len(out), "models,", sum(1 for v in out.values() if v["photo"]), "with a photo")
