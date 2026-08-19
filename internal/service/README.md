# Services / use cases

Tempat workflow aplikasi yang berorientasi pada capability. Jangan membuat
semua subfolder sejak awal; capability package dibuat saat workflow pertamanya
mulai diimplementasikan.

```text
internal/service/
  order/
  payment/
  webhook/
```

Setiap service mendeklarasikan interface berdasarkan operasi yang benar-benar
dikonsumsinya. Service boleh mengimpor entity dan standard library, tetapi
tidak boleh mengimpor Gin, GORM, PostgreSQL, Viper, atau SDK provider.
