#!/usr/bin/env python3
"""Reference-side vCard adapter for the TEST-08 differential suite (issue #680).

The neutral model is the common comparison surface: TEST-03's semanticequal
compares two contactmodel.Record values, so this script must translate the
neutral JSON shape (docs/adrs/0001) to and from a vobject vCard. This file is
the *reference* half of the differential property — it is an independent,
third reading of RFC 6350/2426/9554/9555 layered on top of vobject 0.9.9,
deliberately written WITHOUT any knowledge of our vcard3/vcard4 adapters
(a shared wrong reading of the spec is exactly the failure mode #680 exists
to catch). It is test-only tooling; it ships with the suite, not with the
product.

Invocation (one of):
  vcard_ref.py --to-format 3.0   # neutral Record JSON on stdin -> vCard 3.0 on stdout
  vcard_ref.py --to-format 4.0   # neutral Record JSON on stdin -> vCard 4.0 on stdout
  vcard_ref.py --to-neutral      # vCard on stdin -> neutral Record JSON on stdout
  vcard_ref.py --self-test       # internal sanity check; exit 0 on success

Errors: parse failures are hard errors (exit 1 with a message on stderr).
Unrepresentable neutral fields are simply not emitted (this script is
lossy by design where the 7-slot vCard shape cannot carry the extended
9554 surface); the differential harness decides, per corpus entry, whether
a loss is a reference-side limitation (pinned) or our bug (fails).
"""

import json
import sys

import vobject  # pip-pinned to 0.9.9; see docs/development/testing.md

from vobject.vcard import Name as VName, Address as VAddress

# ---------------------------------------------------------------------------
# neutral model -> vobject
# ---------------------------------------------------------------------------


def _name_components(n):
    """Return (family, given, additional, prefix, suffix) lists from a
    neutral name, per RFC 6350's five N slots (the extended 9554 kinds are
    not representable in the 7-slot N shape)."""
    family, given, additional, prefix, suffix = [], [], [], [], []
    for comp in (n or {}).get("components") or []:
        kind, value = comp.get("kind"), comp.get("value", "")
        if kind == "surname":
            family.append(value)
        elif kind == "given":
            given.append(value)
        elif kind == "given2":
            additional.append(value)
        elif kind == "title":
            prefix.append(value)
        elif kind in ("credential", "generation"):
            suffix.append(value)
        # separator / surname2 / phonetic-only components have no slot
    return family, given, additional, prefix, suffix


def _join(vals):
    return " ".join(vals) if vals else ""


def _add_line(card, name, value, params=None):
    if value == "" and not params:
        return
    line = card.add(name)
    line.value = value
    if params:
        for k, vals in params.items():
            line.params[k] = list(vals)
    return line


def _type_params(contexts, version):
    """Map neutral contexts -> vCard TYPE parameter values."""
    if not contexts:
        return []
    toks = []
    for ctx in contexts:
        if ctx == "private":
            toks.append("home")
        elif ctx == "work":
            toks.append("work")
        elif ctx in ("billing", "delivery"):
            toks.append(ctx)
    return toks


def _phone_types(features):
    toks = []
    for f in features or []:
        toks.append("mobile" if f == "cell" else f)
    return toks


def _pref_params(pref):
    if pref is None:
        return {}
    return {"PREF": [str(pref)]}


def _add_structured(card, name, parts):
    """Add a structured (;-separated) property from a list of slot strings."""
    line = card.add(name)
    line.value = parts
    line.isNative = True
    return line


def _build_org(value):
    # vobject ORG is a plain ;-list (OrgBehavior keeps a native list)
    return value


def neutral_to_vobject(rec, version):
    card = vobject.newFromBehavior("vCard")
    card.add("version").value = version
    c = rec.get("card") or {}

    if c.get("uid"):
        card.add("uid").value = c["uid"]
    if c.get("prodId"):
        _add_line(card, "prodid", c["prodId"])
    if (c.get("updated") or {}).get("utc"):
        _add_line(card, "rev", c["updated"]["utc"])
    if (c.get("created") or {}).get("utc"):
        _add_line(card, "created", _to_vcard_ts(c["created"]["utc"]))
    if c.get("language") and version == "4.0":
        _add_line(card, "language", c["language"])
    if c.get("kind") and version == "4.0":
        _add_line(card, "kind", c["kind"])

    name = c.get("name") or {}
    full = name.get("full") or ""
    comps = name.get("components") or []
    # vCard requires an FN property; when the neutral record has no full name,
    # derive one from the components like our exporters do (an empty FN keeps
    # a truly-anonymous card buildable). Added directly: _add_line skips empty
    # values, and an empty FN must still be present for vobject's validator.
    fn_value = full
    if not fn_value and comps:
        fn_value = " ".join(comp.get("value", "") for comp in comps if comp.get("value"))
    fn_line = card.add("fn")
    fn_line.value = fn_value
    if comps:
        family, given, additional, prefix, suffix = _name_components(name)
        n = card.add("n")
        n.value = VName(family=_join(family), given=_join(given),
                        additional=_join(additional), prefix=_join(prefix),
                        suffix=_join(suffix))
        n.isNative = True
        sort_as = name.get("sortAs")
        if sort_as:
            # SORT-AS has the same five-slot structure as N.
            sa = [sort_as.get("surname", ""), sort_as.get("given", ""),
                  sort_as.get("given2", ""), sort_as.get("title", ""),
                  sort_as.get("credential", "")]
            if any(sa):
                n.params["SORT-AS"] = sa

    for nn in c.get("nicknames") or []:
        if not nn.get("name"):
            continue
        params = _pref_params(nn.get("pref"))
        # One NICKNAME property per entry (RFC 6350 allows either; a single
        # comma-joined property would have its delimiter commas escaped by
        # the serializer and read back as one value).
        _add_line(card, "nickname", nn["name"], params)

    for org in c.get("organizations") or []:
        parts = [org.get("name", "")]
        parts += [u.get("name", "") for u in org.get("units") or []]
        if not any(parts):
            continue
        line = card.add("org")
        line.value = parts
        line.isNative = True

    for title in c.get("titles") or []:
        if not title.get("name"):
            continue
        # ROLE is representable in both 3.0 and 4.0 per the correspondence
        # oracle's v3 ROLE row (RFC 2426-era practice).
        prop = "role" if title.get("kind") == "role" else "title"
        _add_line(card, prop, title["name"])

    for email in c.get("emails") or []:
        if not email.get("address"):
            continue
        params = {}
        types = _type_params(email.get("contexts"), version)
        if types:
            params["TYPE"] = types
        params.update(_pref_params(email.get("pref")))
        _add_line(card, "email", email["address"], params)

    for phone in c.get("phones") or []:
        if not phone.get("number"):
            continue
        params = {}
        types = _type_params(phone.get("contexts"), version) + _phone_types(phone.get("features"))
        if types:
            params["TYPE"] = types
        params.update(_pref_params(phone.get("pref")))
        _add_line(card, "tel", phone["number"], params)

    for svc in c.get("imppAddresses") or []:
        if svc.get("uri"):
            params = {}
            if svc.get("service"):
                params["SERVICE-TYPE"] = [svc["service"]]
            if svc.get("user"):
                params["USERNAME"] = [svc["user"]]
            params.update(_pref_params(svc.get("pref")))
            _add_line(card, "impp", svc["uri"], params)
    for svc in c.get("socialProfiles") or []:
        if version != "4.0":
            # v3 has no native SOCIALPROFILE; the oracle maps it to
            # X-SOCIALPROFILE + X-SERVICE-TYPE, which this reference leg does
            # not emit (out of the reference's fidelity scope).
            continue
        params = {}
        if svc.get("service"):
            params["SERVICE-TYPE"] = [svc["service"]]
        if svc.get("uri"):
            value = svc["uri"]
            if svc.get("user"):
                params["USERNAME"] = [svc["user"]]
        elif svc.get("user"):
            # user-only profile: the value IS the username, flagged VALUE=text
            # (the convention our exporter and importer share).
            value = svc["user"]
            params["VALUE"] = ["text"]
        else:
            continue
        params.update(_pref_params(svc.get("pref")))
        _add_line(card, "socialprofile", value, params)

    for adr in c.get("addresses") or []:
        comps = {comp["kind"]: comp.get("value", "") for comp in adr.get("components") or []}
        box = comps.get("postOfficeBox", "")
        extended = "\\n".join(v for k in ("room", "apartment", "floor", "building") if (v := comps.get(k)))
        street = " ".join(v for k in ("number", "name", "block", "direction", "landmark", "subdistrict", "district") if (v := comps.get(k)))
        a = VAddress(box=box, extended=extended, street=street,
                     city=comps.get("locality", ""), region=comps.get("region", ""),
                     code=comps.get("postcode", ""), country=comps.get("country", ""))
        adr_line = card.add("adr")
        adr_line.value = a
        adr_line.isNative = True
        params = {}
        types = _type_params(adr.get("contexts"), version)
        if types:
            params["TYPE"] = types
        params.update(_pref_params(adr.get("pref")))
        if adr.get("countryCode"):
            params["CC"] = [adr["countryCode"]]
        if adr.get("timeZone"):
            params["TZ"] = [adr["timeZone"]]
        if adr.get("full") and version == "4.0":
            params["LABEL"] = [adr["full"]]
        if params:
            adr_line.params.update(params)
        if adr.get("coordinates"):
            _add_line(card, "geo", adr["coordinates"].replace("geo:", ""))
        if adr.get("full") and version == "3.0":
            _add_line(card, "label", adr["full"])

    for ann in c.get("anniversaries") or []:
        kind = ann.get("kind")
        date = ann.get("date") or {}
        if kind == "birth":
            if date.get("partial"):
                _add_anniv(card, "bday", _partial(date["partial"]))
            elif date.get("timestamp"):
                _add_line(card, "bday", date["timestamp"])
        elif kind == "death":
            if date.get("partial"):
                _add_anniv(card, "deathdate", _partial(date["partial"]))
            elif date.get("timestamp"):
                _add_line(card, "deathdate", date["timestamp"])
        elif kind == "wedding":
            if date.get("partial"):
                _add_anniv(card, "anniversary", _partial(date["partial"]))
            elif date.get("timestamp"):
                _add_line(card, "anniversary", date["timestamp"])
        place = ann.get("place")
        if place and kind in ("birth", "death"):
            prop = "birthplace" if kind == "birth" else "deathplace"
            _add_line(card, prop, _place_text(place))

    sta = c.get("speakToAs") or {}
    for g in sta.get("grammaticalGenders") or []:
        params = {}
        if g.get("language") and version == "4.0":
            params["LANGUAGE"] = [g["language"]]
        _add_line(card, "gramgender", g.get("value", ""), params)
    for p in sta.get("pronouns") or []:
        params = _pref_params(p.get("pref"))
        _add_line(card, "pronouns", p.get("pronouns", ""), params)

    for pi in c.get("personalInfo") or []:
        prop = {"expertise": "expertise", "hobby": "hobby", "interest": "interest"}.get(pi.get("kind"))
        if not prop or version == "3.0":
            continue
        params = {}
        if pi.get("level"):
            params["LEVEL"] = [pi["level"]]
        if pi.get("label"):
            params["LABEL"] = [pi["label"]]
        if pi.get("listAs") is not None:
            params["INDEX"] = [str(pi["listAs"])]
        _add_line(card, prop, pi.get("value", ""), params)

    for note in c.get("notes") or []:
        if note.get("note") == "":
            continue
        params = {}
        if note.get("created"):
            params["CREATED"] = [note["created"].get("utc", "")]
        author = note.get("author") or {}
        if author.get("name"):
            params["AUTHOR-NAME"] = [author["name"]]
        if author.get("uri"):
            params["AUTHOR"] = [author["uri"]]
        _add_line(card, "note", note["note"], params)

    if c.get("keywords"):
        _add_line(card, "categories", list(c["keywords"]))

    for res in c.get("links") or []:
        if res.get("uri"):
            _add_line(card, "url", res["uri"], _pref_params(res.get("pref")))
    for res in c.get("contactUris") or []:
        if res.get("uri"):
            _add_line(card, "contact-uri", res["uri"], _pref_params(res.get("pref")))
    for res in c.get("calendars") or []:
        if res.get("uri"):
            _add_line(card, "caluri", res["uri"])
    for res in c.get("freeBusyUrls") or []:
        if res.get("uri"):
            _add_line(card, "fburl", res["uri"])
    for res in c.get("schedulingAddresses") or []:
        if res.get("uri"):
            _add_line(card, "caladruri", res["uri"])
    for res in c.get("cryptoKeys") or []:
        if res.get("uri"):
            _add_line(card, "key", res["uri"])
    for res in c.get("directories") or []:
        if not res.get("uri"):
            continue
        prop = "org-directory" if res.get("kind") == "directory" else "source"
        _add_line(card, prop, res["uri"])
    for m in c.get("media") or []:
        prop = {"photo": "photo", "logo": "logo", "sound": "sound"}.get(m.get("kind"))
        if prop and m.get("uri"):
            _add_line(card, prop, m["uri"])
    for rel in c.get("relatedTo") or []:
        params = {}
        if rel.get("relations"):
            params["TYPE"] = rel["relations"]
        _add_line(card, "related", rel.get("target", ""), params)
    for m in c.get("members") or []:
        _add_line(card, "member", m)
    for lp in c.get("preferredLanguages") or []:
        params = {}
        types = _type_params(lp.get("contexts"), version)
        if types:
            params["TYPE"] = types
        params.update(_pref_params(lp.get("pref")))
        _add_line(card, "lang", lp.get("language", ""), params)

    return card


def _partial(p):
    if p.get("year") is not None and p.get("month") is not None and p.get("day") is not None:
        return "%04d-%02d-%02d" % (p["year"], p["month"], p["day"])
    if p.get("month") is not None and p.get("day") is not None:
        return "--%02d-%02d" % (p["month"], p["day"])
    if p.get("month") is not None:
        return "--%02d" % p["month"]
    if p.get("year") is not None:
        return "%04d" % p["year"]
    return ""


def _add_anniv(card, prop, value):
    if value:
        _add_line(card, prop, value)


def _place_text(place):
    full = place.get("full")
    if full:
        return full
    coords = place.get("coordinates")
    if coords:
        return coords.replace("geo:", "")
    comps = place.get("components") or []
    if comps:
        return ", ".join(c.get("value", "") for c in comps if c.get("value"))
    return ""


# ---------------------------------------------------------------------------
# vobject -> neutral model
# ---------------------------------------------------------------------------

def _prop_value(card, name):
    """Return the raw .value of the first property named name, or None."""
    lines = getattr(card, name.lower() + "_list", None)
    if lines:
        return lines[0].value
    return None


def _prop_params(card, name):
    lines = getattr(card, name.lower() + "_list", None)
    if not lines:
        return []
    return lines


def _lines(v, prop):
    """Return the ContentLines of a property, however vobject names it:
    registered behaviors expose <name>_list attributes; unregistered
    properties (CONTACT-URI, ORG-DIRECTORY, ...) only live in contents under
    the lowercased property name."""
    attr = getattr(v, prop.lower() + "_list", None)
    if attr is not None:
        return attr
    return v.contents.get(prop.lower()) or []


def _first(v, prop):
    lines = _lines(v, prop)
    return lines[0] if lines else None


def vobject_to_neutral(v):
    card = {}
    uid = _first(v, "UID")
    _set(card, "uid", uid.value if uid else "")
    kind = _first(v, "KIND")
    _set(card, "kind", kind.value if kind else "")
    prodid = _first(v, "PRODID")
    _set(card, "prodId", prodid.value if prodid else "")
    language = _first(v, "LANGUAGE")
    _set(card, "language", language.value if language else "")
    rev = _first(v, "REV")
    if rev:
        card["updated"] = {"utc": _normalize_ts(rev.value)}
    created = _first(v, "CREATED")
    if created:
        card["created"] = {"utc": _normalize_ts(created.value)}

    name = {}
    fn = _first(v, "FN")
    if fn:
        name["full"] = fn.value
    n = _first(v, "N")
    if n:
        nval = n.value
        comps = []
        if isinstance(nval, VName):
            for slot, kind in ((nval.prefix, "title"), (nval.given, "given"), (nval.additional, "given2"), (nval.family, "surname"), (nval.suffix, "credential")):
                comps += [{"kind": kind, "value": val} for val in _as_list(slot)]
        else:
            parts = str(nval).split()
            # unparsed structured fallback — best-effort
            comps = [{"kind": "surname", "value": parts[0]}] if parts else []
        if comps:
            name["components"] = comps
        if n.params:
            if "SORT-AS" in n.params and n.params["SORT-AS"]:
                sa = n.params["SORT-AS"]
                name["sortAs"] = {"surname": sa[0] if len(sa) > 0 else ""}
    # Phonetic readings live on a separate N variant line (ALTID + PHONETIC +
    # SCRIPT params, RFC 9554): scan every N line for the params, since the
    # variant is not the first N.
    for nline in _lines(v, "N"):
        if not nline.params:
            continue
        phon = nline.params.get("PHONETIC")
        script = nline.params.get("SCRIPT")
        if phon or script:
            name["phoneticSystem"] = phon[0] if phon else ""
            name["phoneticScript"] = script[0] if script else ""
    if name:
        card["name"] = name

    nicknames = []
    for nl in _lines(v, "NICKNAME"):
        entry = {"name": nl.value}
        contexts = _contexts_from_types(nl.params)
        if contexts:
            entry["contexts"] = contexts
        pref = nl.params.get("PREF")
        if pref and pref[0].isdigit():
            entry["pref"] = int(pref[0])
        nicknames.append(entry)
    if nicknames:
        card["nicknames"] = nicknames

    orgs = []
    for o in _lines(v, "ORG"):
        parts = o.value if isinstance(o.value, list) else str(o.value).split(";")
        if parts and parts[0]:
            org = {"name": parts[0]}
            if len(parts) > 1:
                org["units"] = [{"name": u} for u in parts[1:] if u]
            orgs.append(org)
    if orgs:
        card["organizations"] = orgs

    titles = []
    for t in _lines(v, "TITLE"):
        titles.append({"name": t.value, "kind": "title"})
    for t in _lines(v, "ROLE"):
        titles.append({"name": t.value, "kind": "role"})
    if titles:
        card["titles"] = titles

    emails = []
    for e in _lines(v, "EMAIL"):
        entry = {"address": e.value}
        _apply_common(e, entry)
        emails.append(entry)
    if emails:
        card["emails"] = emails

    phones = []
    for t in _lines(v, "TEL"):
        entry = {"number": t.value}
        feats = _phone_features(t.params)
        if feats:
            entry["features"] = feats
        _apply_common(t, entry)
        phones.append(entry)
    if phones:
        card["phones"] = phones

    impps = [_online(t) for t in _lines(v, "IMPP")]
    if impps:
        card["imppAddresses"] = impps
    socials = [_online(t) for t in _lines(v, "SOCIALPROFILE")]
    if socials:
        card["socialProfiles"] = socials

    addrs = []
    for a in _lines(v, "ADR"):
        addrs.append(_address_to_neutral(a))
    if addrs:
        card["addresses"] = addrs

    annivs = []
    bday = _first(v, "BDAY")
    if bday:
        annivs.append(_anniv("birth", bday))
    deathdate = _first(v, "DEATHDATE")
    if deathdate:
        annivs.append(_anniv("death", deathdate))
    anniversary = _first(v, "ANNIVERSARY")
    if anniversary:
        annivs.append(_anniv("wedding", anniversary))
    birthplace = _first(v, "BIRTHPLACE")
    if birthplace:
        _merge_place(annivs, "birth", birthplace)
    deathplace = _first(v, "DEATHPLACE")
    if deathplace:
        _merge_place(annivs, "death", deathplace)
    if annivs:
        card["anniversaries"] = annivs

    gs = [{"value": g.value, "language": (g.params.get("LANGUAGE") or [""])[0]} for g in _lines(v, "GRAMGENDER")]
    if gs:
        card["speakToAs"] = {"grammaticalGenders": gs}
    ps = [{"pronouns": p.value} for p in _lines(v, "PRONOUNS")]
    if ps:
        if "speakToAs" not in card:
            card["speakToAs"] = {}
        card["speakToAs"]["pronouns"] = ps

    pis = []
    for prop, kind in (("EXPERTISE", "expertise"), ("HOBBY", "hobby"), ("INTEREST", "interest")):
        for line in _lines(v, prop):
            entry = {"kind": kind, "value": line.value}
            if line.params.get("LEVEL"):
                entry["level"] = line.params["LEVEL"][0]
            if line.params.get("INDEX") and line.params["INDEX"][0].isdigit():
                entry["listAs"] = int(line.params["INDEX"][0])
            pis.append(entry)
    if pis:
        card["personalInfo"] = pis

    notes = []
    for n in _lines(v, "NOTE"):
        entry = {"note": n.value}
        created = n.params.get("CREATED")
        if created:
            entry["created"] = {"utc": _normalize_ts(created[0])}
        author_name = n.params.get("AUTHOR-NAME")
        author_uri = n.params.get("AUTHOR")
        if author_name or author_uri:
            a = {}
            if author_name:
                a["name"] = author_name[0]
            if author_uri:
                a["uri"] = author_uri[0]
            entry["author"] = a
        notes.append(entry)
    if notes:
        card["notes"] = notes

    categories = _first(v, "CATEGORIES")
    if categories:
        card["keywords"] = _split_list(categories.value)

    links = [_resource(x) for x in _lines(v, "URL")]
    if links:
        card["links"] = links
    contacturis = [_resource(x) for x in _lines(v, "CONTACT-URI")]
    if contacturis:
        card["contactUris"] = contacturis
    calendars = [_resource(x) for x in _lines(v, "CALURI")]
    if calendars:
        card["calendars"] = calendars
    freebusy = [_resource(x) for x in _lines(v, "FBURL")]
    if freebusy:
        card["freeBusyUrls"] = freebusy
    scheduling = [_resource(x) for x in _lines(v, "CALADRURI")]
    if scheduling:
        card["schedulingAddresses"] = scheduling
    keys = [_resource(x) for x in _lines(v, "KEY")]
    if keys:
        card["cryptoKeys"] = keys
    related = [{"target": r.value, "relations": _split_list(r.params.get("TYPE") or [])} for r in _lines(v, "RELATED")]
    if related:
        card["relatedTo"] = related
    members = [m.value for m in _lines(v, "MEMBER")]
    if members:
        card["members"] = members
    langs = [_lang(l) for l in _lines(v, "LANG")]
    if langs:
        card["preferredLanguages"] = langs

    media = []
    for p in _lines(v, "PHOTO"):
        media.append({"kind": "photo", "uri": p.value})
    for p in _lines(v, "LOGO"):
        media.append({"kind": "logo", "uri": p.value})
    for p in _lines(v, "SOUND"):
        media.append({"kind": "sound", "uri": p.value})
    if media:
        card["media"] = media

    dirs = []
    for s in _lines(v, "SOURCE"):
        dirs.append({"kind": "entry", "uri": s.value})
    for s in _lines(v, "ORG-DIRECTORY"):
        dirs.append({"kind": "directory", "uri": s.value})
    if dirs:
        card["directories"] = dirs

    rec = {"card": card}
    return rec


def _set(d, k, v):
    if v not in (None, ""):
        d[k] = v


def _as_list(val):
    if val is None:
        return []
    if isinstance(val, (list, tuple)):
        return [x for x in val if x]
    # A slot holds one logical value (vobject may have joined a multi-value
    # list with a space); never whitespace-split a string slot, or a long
    # component containing spaces fragments into many.
    return [val] if val != "" else []


def _normalize_ts(ts):
    """Normalize a vCard timestamp to the neutral model's RFC3339 instant.
    vobject emits basic-format timestamps (20261122T151823Z) on parse; the
    neutral model and the ts_rfc3339 comparison transform expect RFC3339."""
    if not ts:
        return ts
    s = str(ts).strip()
    # vCard basic format: YYYYMMDDTHHMMSSZ
    if len(s) == 16 and s.endswith("Z"):
        try:
            return "%s-%s-%sT%s:%s:%sZ" % (s[0:4], s[4:6], s[6:8], s[9:11], s[11:13], s[13:15])
        except Exception:  # pragma: no cover — guarded, malformed input falls through
            return ts
    return ts


def _to_vcard_ts(rfc3339):
    """RFC3339 -> vCard basic-format timestamp (the form our exporters and
    vobject both understand)."""
    if not rfc3339:
        return rfc3339
    s = str(rfc3339)
    if len(s) == 20 and s[10] == "T" and s.endswith("Z"):
        return "%s%s%sT%s%s%sZ" % (s[0:4], s[5:7], s[8:10], s[11:13], s[14:16], s[17:19])
    return rfc3339


def _contexts_from_types(params):
    contexts = []
    for t in params.get("TYPE") or []:
        if t.lower() == "home":
            contexts.append("private")
        elif t.lower() == "work":
            contexts.append("work")
    return contexts


def _split_list(text):
    # vCard comma-lists (NICKNAME, CATEGORIES, TYPE) split on commas.
    if isinstance(text, list):
        return text
    return [t.strip() for t in text.split(",") if t.strip()]


def _apply_common(line, entry):
    types = line.params.get("TYPE") or []
    contexts = []
    for t in types:
        if t.lower() == "home":
            contexts.append("private")
        elif t.lower() == "work":
            contexts.append("work")
    if contexts:
        entry["contexts"] = contexts
    pref = line.params.get("PREF")
    if pref and pref[0].isdigit():
        entry["pref"] = int(pref[0])
    label = line.params.get("LABEL")
    if label:
        entry["label"] = label[0]


def _phone_features(params):
    feats = []
    for t in params.get("TYPE") or []:
        tl = t.lower()
        if tl in ("cell", "fax", "video", "pager", "text", "textphone", "main-number"):
            feats.append("cell" if tl == "cell" else tl)
        elif tl == "mobile":
            feats.append("cell")
    return feats


def _online(line):
    entry = {}
    if line.params.get("SERVICE-TYPE"):
        entry["service"] = line.params["SERVICE-TYPE"][0]
    if line.params.get("USERNAME"):
        entry["user"] = line.params["USERNAME"][0]
    entry["uri"] = line.value
    _apply_common(line, entry)
    return entry


def _address_to_neutral(a):
    v = a.value
    comps = []
    if isinstance(v, VAddress):
        slots = (("postOfficeBox", v.box), ("apartment", v.extended), ("name", v.street),
                 ("locality", v.city), ("region", v.region), ("postcode", v.code),
                 ("country", v.country))
        for kind, val in slots:
            # A slot holds one logical value (vobject may have joined a
            # multi-value list with a space/newline); never whitespace-split.
            for piece in (val if isinstance(val, (list, tuple)) else [val]):
                if piece:
                    comps.append({"kind": kind, "value": piece})
    else:
        comps = [{"kind": "name", "value": str(v)}] if v else []
    entry = {"components": comps}
    if a.params.get("CC"):
        entry["countryCode"] = a.params["CC"][0]
    if a.params.get("TZ"):
        entry["timeZone"] = a.params["TZ"][0]
    if a.params.get("LABEL"):
        entry["full"] = a.params["LABEL"][0]
    _apply_common(a, entry)
    return entry


def _anniv(kind, line):
    value = line.value
    # vobject parses BDAY/ANNIVERSARY/DEATHDATE into a datetime.date/datetime
    # for full dates (DATE/DATE-TIME behavior), leaving partial dates as
    # strings. Handle both.
    if isinstance(value, str):
        text = value
    else:
        text = _date_to_vcard(value)
    date = {}
    if "T" in text or text.endswith("Z"):
        date = {"timestamp": _normalize_ts(text)}
    else:
        p = _parse_partial(text)
        if p:
            date = {"partial": p}
    return {"kind": kind, "date": date}


def _date_to_vcard(value):
    # datetime.date / datetime.datetime -> vCard date text
    try:
        from datetime import date, datetime
        if isinstance(value, datetime):
            return value.strftime("%Y%m%dT%H%M%SZ")
        if isinstance(value, date):
            return value.strftime("%Y%m%d")
    except Exception:  # pragma: no cover
        return str(value)
    return str(value)


def _parse_partial(text):
    # RFC 6350 partial dates: YYYY / YYYY-MM / YYYY-MM-DD / --MM-DD / --MM
    p = {}
    if text.startswith("--"):
        nums = text[2:].split("-")
        if len(nums) >= 1 and nums[0].isdigit():
            p["month"] = int(nums[0])
        if len(nums) >= 2 and nums[1].isdigit():
            p["day"] = int(nums[1])
    else:
        nums = text.split("-")
        if nums[0].isdigit():
            p["year"] = int(nums[0])
        if len(nums) >= 2 and nums[1].isdigit():
            p["month"] = int(nums[1])
        if len(nums) >= 3 and nums[2].isdigit():
            p["day"] = int(nums[2])
    return p or None


def _merge_place(annivs, kind, line):
    text = _place_text({"full": line.value})
    for a in annivs:
        if a["kind"] == kind:
            a["place"] = {"full": text}
            return
    annivs.append({"kind": kind, "date": {}, "place": {"full": text}})


def _resource(line):
    entry = {"uri": line.value}
    _apply_common(line, entry)
    return entry


def _lang(line):
    entry = {"language": line.value}
    _apply_common(line, entry)
    return entry


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

def main():
    mode = sys.argv[1] if len(sys.argv) > 1 else ""
    if mode == "--self-test":
        _self_test()
        return 0
    if mode == "--to-format":
        if len(sys.argv) < 3:
            print("vcard_ref.py --to-format <3.0|4.0>", file=sys.stderr)
            return 2
        version = sys.argv[2]
        rec = json.load(sys.stdin)
        try:
            card = neutral_to_vobject(rec, version)
            sys.stdout.write(card.serialize())
        except Exception as exc:  # noqa: BLE001 — a reference that cannot build a card must fail loudly, not emit partial output
            print("vcard_ref: neutral->vCard %s failed: %s: %s" % (version, type(exc).__name__, exc), file=sys.stderr)
            return 1
        return 0
    if mode == "--to-neutral":
        data = sys.stdin.read()
        try:
            v = vobject.readOne(data)
            json.dump(_coerce_bytes(vobject_to_neutral(v)), sys.stdout)
        except Exception as exc:  # noqa: BLE001
            print("vcard_ref: vCard->neutral failed: %s: %s" % (type(exc).__name__, exc), file=sys.stderr)
            return 1
        return 0
    print(__doc__, file=sys.stderr)
    return 2


def _coerce_bytes(obj):
    """Deep-coerce bytes (which vobject produces for e.g. base64 PHOTO
    values) to str so json.dump can serialize the neutral record."""
    if isinstance(obj, bytes):
        return obj.decode("utf-8", errors="replace")
    if isinstance(obj, dict):
        return {k: _coerce_bytes(v) for k, v in obj.items()}
    if isinstance(obj, list):
        return [_coerce_bytes(v) for v in obj]
    return obj


def _self_test():
    """Cheap internal wiring check: parse a minimal card and rebuild it, so a
    broken script or a missing/wrong vobject version fails fast in the Go
    harness probe rather than mid-corpus."""
    v = vobject.readOne("BEGIN:VCARD\nVERSION:4.0\nFN:Self Test\nEND:VCARD\n")
    assert v.fn.value == "Self Test", v.fn.value
    # The structured-value behaviors are the most fragile surface; exercise them.
    card = vobject.newFromBehavior("vCard")
    card.add("version").value = "4.0"
    card.add("fn").value = "Jane Doe"
    n = card.add("n")
    n.value = VName(family="Doe", given="Jane")
    a = card.add("adr")
    a.value = VAddress(street="1 Main St", city="Springfield", code="62701", country="US")
    card.add("email").value = "j@example.com"
    back = vobject.readOne(card.serialize())
    assert back.n.value.family == "Doe", back.n.value
    assert back.adr.value.city == "Springfield", back.adr.value


if __name__ == "__main__":
    sys.exit(main())
