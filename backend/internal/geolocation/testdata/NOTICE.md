# GeoIP test fixture

`GeoLite2-City-Test.mmdb` is the fake example database from the official
[MaxMind-DB test data repository](https://github.com/maxmind/MaxMind-DB/tree/c454a21f31fd17f2843f16094e9afb3a14a42078/test-data),
revision `c454a21f31fd17f2843f16094e9afb3a14a42078`.

SHA-256: `8b6b4983c0d6c61d293f6da4a01f037c22b7c4457621e64289220dc86cd2506f`

`GeoIP2-City-Test.mmdb` is included from the same upstream revision for the
same parser coverage.

SHA-256: `1394af009121820d43b620fe19fc4b6f09141698091c00f70f225a982389035`

It is used only to verify reader/parser behavior. Its records are fictional and
must not be used for production geolocation or to validate current real-world
IP assignments. The upstream repository is distributed under Apache-2.0 or MIT;
the MIT license text is included alongside the fixture.
