# Laporan Pengujian API Web Pinguin

**Tanggal:** 23 November 2026  
**Metode:** Postman CLI dengan request HTTP dan assertion  
**API:** `http://127.0.0.1:8080`  
**Database:** PostgreSQL 17 di Docker, `127.0.0.1:5432`  
**Data uji:** direktori fixture sementara berisi `fixture.iso` (23 byte)

## Hasil pengujian

| Pengujian | Hasil yang diharapkan | Hasil aktual | Kesesuaian |
|---|---|---|---|
| `GET /health/live` | HTTP `200`; JSON menunjukkan status `ok` | HTTP `200`; `{"status":"ok"}` | Sesuai — lulus |
| `GET /health/ready` | HTTP `200` saat PostgreSQL dan direktori listing tersedia | HTTP `200`; status `ok` | Sesuai — lulus |
| Listing root, `GET /api/v1/listing?page=1&page_size=20` | HTTP `200`; daftar memuat file di direktori listing | HTTP `200`; `fixture.iso` muncul dengan ukuran 23 byte | Sesuai — lulus |
| Pagination tidak valid, `GET /api/v1/listing?page=0` | HTTP `400`; kode error `invalid_query` | HTTP `400`; kode `invalid_query`; kedua assertion Postman lulus | Sesuai — lulus |
| Unduh file, `GET /api/v1/download?path=fixture.iso` | HTTP `200`; konten sama dengan file sumber | HTTP `200`; konten `postman-fixture-content` diterima utuh | Sesuai — lulus setelah perbaikan pencatatan event |
| Unduh sebagian, `Range: bytes=0-5` | HTTP `206`; hanya 6 byte pertama dikirim | HTTP `206`; body `postma` (6 byte) | Sesuai — lulus |
| Path traversal, `GET /api/v1/listing?path=..%2F..%2Fbackend` | HTTP `400`; path di luar root ditolak | HTTP `400`; kode error `invalid_path`; kedua assertion Postman lulus | Sesuai — lulus |
| Ringkasan, `GET /api/v1/stats/summary?period=24h` | HTTP `200`; ringkasan mencakup request unduhan | HTTP `200`; 2 request, 1 selesai, 29 byte, file populer `fixture.iso` | Sesuai — lulus |
| Timeseries, `GET /api/v1/stats/timeseries?period=24h` | HTTP `200`; data berupa bucket statistik | HTTP `200`; 1 bucket berisi 2 request, 1 selesai, 29 byte | Sesuai — lulus |

## Temuan dan perbaikan

Pengujian awal menemukan bahwa unduhan dapat merespons HTTP `200`, tetapi
event-nya gagal disimpan karena satu parameter SQL dipakai untuk kolom UUID dan
teks. Query pada
[`backend/internal/analytics/store.go`](../backend/internal/analytics/store.go)
kemudian diberi cast eksplisit. Setelah perbaikan, pengujian ulang berhasil
mencatat kedua request unduhan. Worker mengagregasi 2 event ke 1 bucket.

Pada request yang memang mengharapkan HTTP `400`, Postman CLI memberi exit code
non-zero karena responsnya non-2xx. Assertion untuk status dan kode error tetap
lulus; hasil tersebut sesuai dengan perilaku API yang diuji.

## Kesimpulan

Seluruh perilaku akhir yang diuji sesuai dengan hasil yang diharapkan. PostgreSQL
Docker sehat di `127.0.0.1:5432` dan migrasi database sudah diterapkan.

Listing dan unduhan diuji memakai fixture sementara, bukan direktori mirror
sebenarnya karena nilai `LISTING_DIR` belum ditentukan. API dihentikan setelah
pengujian agar fixture sementara tidak digunakan sebagai listing aktif;
container PostgreSQL tetap berjalan.
