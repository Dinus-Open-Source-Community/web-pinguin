ALTER TABLE geo_regions ALTER COLUMN latitude DROP NOT NULL;
ALTER TABLE geo_regions ALTER COLUMN longitude DROP NOT NULL;

INSERT INTO geo_regions (country_code, region_code, country_name, region_name)
VALUES ('ID', 'COUNTRY', 'Indonesia', 'Indonesia')
ON CONFLICT (country_code, region_code) DO NOTHING;
