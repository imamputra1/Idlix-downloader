import json
import base64
import hashlib
from Crypto.Cipher import AES

# Ini adalah Payload #5 terbaru yang Anda dapatkan dari Scraper (dengan CookieJar)
json_str = '{"ct":"BFArUsXSnuwqR9T\\/UBHR2Zlw3FwLUXi1n+qv2BXkeKsySzqYd\\/2KGPSHLIz86Xq8tuZtHV8\\/9Vf8hb8fdCePbNFu0mczdtIxZ7iZTGUFSJs=","iv":"b0bb3661bd5d7f5cc6d0d6344177cd78","s":"921d2c358aea887d","m":"ANzwnM8BjM8hzM8djM8FzM8VjM8hTM8hjM8dDf5MDfyQDf0EDfxwHMxwXNzwXO8ZTM8hDfwQDf3EDfwMDfzIDf2w3M8JzM8RjM8lTM8ZjM8VTM8NDN8RDfxEDf2MDf5IDf1wnMxwHM8JjM8NzM8NTM8FDN8FjM8dzM"}'
r = "\\x59\\x56\\x4d\\x33\\x4d\\x4f\\x59\\x67\\x54\\x6c\\x6d\\x34\\x32\\x78\\x6c\\x67\\x46\\x32\\x5a\\x4d\\x57\\x7a\\x6a\\x6a\\x6a\\x44\\x30\\x31\\x35\\x47\\x55\\x4d\\x49\\x5a\\x3d\\x4e\\x45\\x4d\\x4d\\x4d\\x4d\\x59\\x4e\\x7a"
e = json.loads(json_str).get('m')

# 1. BONGKAR PASSPHRASE
r_list = [r[i:i + 2] for i in range(2, len(r), 4)]
m_padded = e[::-1] + '=' * (-len(e[::-1]) % 4)
m_decoded = base64.b64decode(m_padded).decode('utf-8')
m_list = m_decoded.split('|')
passphrase = ''
for s in m_list:
    if s.isdigit():
        idx = int(s)
        if idx < len(r_list):
            passphrase += chr(int(r_list[idx], 16))

print("Passphrase HEX :", passphrase.encode('utf-8').hex())

# 2. BONGKAR AES KEY (Hanya menggunakan KDF 32-byte)
json_data = json.loads(json_str)
salt = bytes.fromhex(json_data["s"])
iv = bytes.fromhex(json_data["iv"]) # Menggunakan IV dari JSON
ct = base64.b64decode(json_data["ct"])

concated = passphrase.encode() + salt
res = hashlib.md5(concated).digest()
for _ in range(1, 3):
    res += hashlib.md5(res + concated).digest()
key = res[:32]

# 3. DEKRIPSI
cipher = AES.new(key, AES.MODE_CBC, iv)
dec = cipher.decrypt(ct)

try:
    unpadded = dec[:-dec[-1]]
    final_url = unpadded.decode('utf-8')
    print("\n[HORE!] URL BERHASIL DIDEKRIPSI :", final_url)
except Exception as err:
    print("\n[ZONK!] DEKRIPSI GAGAL (GARBAGE BLOCK):", err)
    print("ALASAN: Peladen MASIH memberikan kita payload palsu (Honeypot).")
