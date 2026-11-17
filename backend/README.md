# Backend

Backend akan menggunakan Go dan Gin. Service API dan worker analytics dijalankan
sebagai proses terpisah agar pemrosesan log tidak mengganggu request dashboard.

## Batas tahap arsitektur

Folder ini sengaja belum berisi source code atau dependency. Endpoint, model data,
retensi, dan aturan privasi yang menjadi dasar implementasi tercantum pada
[`docs/ARCHITECTURE.md`](../docs/ARCHITECTURE.md).

API tidak boleh mengembalikan IP atau event download mentah. Semua data map publik
harus berasal dari tabel agregat.
