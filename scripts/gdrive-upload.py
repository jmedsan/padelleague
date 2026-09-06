#!/usr/bin/env python3
"""Upload a file to Google Drive using a service account.

Usage: gdrive-upload.py <service-account.json> <folder-id> <local-file>

Requires: PyJWT, cryptography (both standard on most systems).
"""
import json, sys, time, os
import jwt
from urllib.request import urlopen, Request
from urllib.parse import urlencode

def get_access_token(sa_path):
    with open(sa_path) as f:
        sa = json.load(f)
    now = int(time.time())
    payload = {
        "iss": sa["client_email"],
        "scope": "https://www.googleapis.com/auth/drive.file",
        "aud": "https://oauth2.googleapis.com/token",
        "iat": now,
        "exp": now + 3600,
    }
    signed = jwt.encode(payload, sa["private_key"], algorithm="RS256")
    data = urlencode({
        "grant_type": "urn:ietf:params:oauth:grant-type:jwt-bearer",
        "assertion": signed,
    }).encode()
    req = Request("https://oauth2.googleapis.com/token", data=data)
    resp = json.loads(urlopen(req, timeout=30).read())
    return resp["access_token"]

def upload(token, folder_id, filepath):
    filename = os.path.basename(filepath)
    metadata = json.dumps({"name": filename, "parents": [folder_id]}).encode()
    with open(filepath, "rb") as f:
        file_data = f.read()

    boundary = b"----BackupBoundary"
    body = (
        b"--" + boundary + b"\r\n"
        b"Content-Type: application/json; charset=UTF-8\r\n\r\n"
        + metadata + b"\r\n"
        b"--" + boundary + b"\r\n"
        b"Content-Type: application/octet-stream\r\n\r\n"
        + file_data + b"\r\n"
        b"--" + boundary + b"--\r\n"
    )
    req = Request(
        "https://www.googleapis.com/upload/drive/v3/files?uploadType=multipart",
        data=body,
        headers={
            "Authorization": f"Bearer {token}",
            "Content-Type": f"multipart/related; boundary={boundary.decode()}",
        },
    )
    resp = json.loads(urlopen(req, timeout=120).read())
    return resp.get("id", "unknown")

if __name__ == "__main__":
    if len(sys.argv) != 4:
        print(f"Usage: {sys.argv[0]} <sa.json> <folder-id> <file>", file=sys.stderr)
        sys.exit(1)
    sa_path, folder_id, filepath = sys.argv[1], sys.argv[2], sys.argv[3]
    token = get_access_token(sa_path)
    file_id = upload(token, folder_id, filepath)
    print(f"uploaded {os.path.basename(filepath)} → {file_id}")
