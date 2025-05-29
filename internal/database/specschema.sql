CREATE TABLE IF NOT EXISTS device_characteristics (
    name VARCHAR(255) PRIMARY KEY,
    hdf5_file VARCHAR(255) NOT NULL,
    dataset VARCHAR(255) NOT NULL,
    indexes JSONB,
    uncertainty DOUBLE PRECISION,
    hash VARCHAR(255),
    time TIMESTAMP WITH TIME ZONE,
    state JSONB,
    uuid UUID DEFAULT gen_random_uuid()
);
