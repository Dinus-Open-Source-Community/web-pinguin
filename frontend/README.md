# Frontend

Frontend akan menggunakan TanStack Router, TanStack Query, TypeScript, dan Vite.

## Batas tahap arsitektur

Folder ini sengaja belum berisi source code atau dependency. Implementasi dimulai
dengan route `/`, `/repositories`, `/statistics`, `/request-mirror`,
`/documentation`, dan `/about` setelah kontrak API pada
[`docs/ARCHITECTURE.md`](../docs/ARCHITECTURE.md) disetujui.

TanStack Query menjadi satu-satunya sumber server-state. Filter repository dan
periode statistik disimpan sebagai search params melalui TanStack Router agar URL
dapat dibagikan dan direfresh tanpa kehilangan konteks.
