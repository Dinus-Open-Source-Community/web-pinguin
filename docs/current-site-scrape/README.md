# Hasil Scrap Situs Pinguin Dinus Saat Ini

**Target:** https://pinguin.dinus.ac.id/  
**Diambil:** 23 November 2026 (waktu Indonesia)  
**Metode:** Scrapling 0.5.1, `scrapling extract get` dengan `--ai-targeted`; halaman
disimpan sebagai Markdown. Hanya halaman publik yang diminta langsung; tidak ada
login, pengunduhan file ISO, atau pengambilan data pribadi.

## Berkas hasil

| Berkas | Halaman | Isi |
|---|---|---|
| [home.md](home.md) | `/` | Beranda, navigasi, distro unggulan, profil, dan kontak. |
| [iso-index.md](iso-index.md) | `/iso/` | Listing direktori ISO; 90 subdirektori pada saat pengambilan. |
| [iso-debian.md](iso-debian.md) | `/iso/debian/` | Folder versi Debian yang tersedia. |
| [iso-debian13.md](iso-debian13.md) | `/iso/debian/debian13/` | Listing file Debian 13 beserta ukuran dan waktu modifikasi yang ditampilkan server. Tidak mengunduh ISO. |
| [iso-ubuntu.md](iso-ubuntu.md) | `/iso/ubuntu/` | Folder keluarga Ubuntu. |
| [iso-tealinuxos.md](iso-tealinuxos.md) | `/iso/tealinuxos/` | Folder TeaLinuxOS. |
| [legacy-linux-debian.md](legacy-linux-debian.md) | `/linux/debian.html` | Halaman informasi Debian lama. |

Pengecekan `robots.txt` dan `sitemap.xml` menghasilkan HTTP 404. Kegagalan
tersebut dicatat sebagai observasi; tidak ada direktori mirror atau file biner
yang diunduh. Hasil Markdown adalah ekstraksi konten, sehingga styling dan
interaksi JavaScript tidak direpresentasikan.

## Ringkasan situs yang diamati

- Beranda menampilkan pilihan distro unggulan, pengenalan TeaLinuxOS, tautan ke
  masing-masing halaman distro, informasi komunitas DOSCOM, alamat, dan kontak.
- Tautan “index semua ISO” menuju `/iso/`, yang pada scrape ini menampilkan 90
  direktori. Jadi ada dua tingkat katalog: pilihan yang dipromosikan beranda
  dan direktori mirror yang jauh lebih lengkap.
- Listing `/iso/` dan subdirektorinya tampil sebagai Apache auto-index, dengan
  nama, waktu modifikasi, dan ukuran file/folder. Contoh Debian menampilkan
  folder versi 5 sampai 13; halaman Debian 13 mencantumkan tiga ISO beserta
  ukuran. Listing yang diambil tidak menampilkan checksum.
- Halaman lama `/linux/debian.html` berisi deskripsi Debian, tetapi bagian
  unduhan yang diekstrak berjudul “Download TeaLinuxOS” dan tidak menampilkan
  tautan ISO yang jelas. Ini menunjukkan potensi konten/template usang; perlu
  pengecekan visual/manual sebelum dianggap sebagai perilaku pasti browser.
- Pada halaman yang diamati, tidak terlihat pencarian katalog terpadu, filter,
  statistik unduhan, monitoring, peta aktivitas, survei request ISO, atau
  dashboard admin.
- Respons 404 untuk `robots.txt`/`sitemap.xml` dan footer auto-index
  mengungkap banner server “Apache/2.4.71 (Debian)” pada halaman error/listing.
  Perilaku header, konfigurasi keamanan, dan server sebenarnya belum diaudit.

## Implikasi untuk rebuild portal

1. Jadikan katalog portal lapisan yang ramah pengguna di atas data mirror yang
   sudah ada; jangan menyalin file ISO ke backend baru.
2. Simpan daftar distro unggulan sebagai metadata terkurasi, tetapi sediakan
   katalog lengkap yang dapat dicari dan difilter. Jangan menganggap 90 entri
   hasil scrape ini sebagai angka permanen.
3. Model metadata minimum yang tampak tersedia: distribusi/kategori, path,
   versi/folder, nama file, ukuran, waktu modifikasi, URL unduhan, dan status
   ketersediaan. Dapatkan checksum dari sumber yang sah sebelum menampilkannya;
   jangan mengarang checksum bila listing tidak menyediakannya.
4. Hindari mengekspos Apache auto-index mentah sebagai pengalaman utama. Backend
   dapat menyinkronkan metadata/file listing dari sumber mirror yang disetujui,
   menyaring item, dan menyajikan API katalog; tautan unduh tetap langsung ke
   mirror.
5. Validasi tautan versi lama dan isi halaman distro. Tampilkan status
   “diperbarui pada” dengan sumber/waktu yang jelas agar entri usang mudah
   dikenali.
6. Hasil scrape ini belum memberi bukti tentang ketersediaan API metadata,
   access log untuk analytics, endpoint health/status, autentikasi admin, atau
   mekanisme perubahan mirror. Semua itu perlu dikonfirmasi dengan operator
   mirror sebelum integrasi.

## Batasan

Scrape ini mengambil beranda, beberapa listing direktori, dan satu halaman
legacy untuk membuat sampel yang relevan dengan katalog. Ini bukan inventaris
seluruh 90 subdirektori maupun crawl isi mirror. Data waktu/ukuran adalah apa yang
ditampilkan origin saat pengambilan; dapat berubah. Jangan gunakan sebagai
snapshot kapasitas atau status mirror permanen.
