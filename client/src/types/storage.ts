export type StorageStatus = {
  configured: boolean;
  enabled: boolean;
  has_secret: boolean;
  provider?: string;
  endpoint?: string;
  bucket?: string;
  region?: string;
  access_key?: string;
  prefix?: string;
  use_ssl?: boolean;
  updated_at?: string;
};

export type StorageForm = {
  provider: string;
  endpoint: string;
  bucket: string;
  region: string;
  access_key: string;
  secret_key: string; // vazio = manter o atual
  prefix: string;
  use_ssl: boolean;
  enabled: boolean;
};
