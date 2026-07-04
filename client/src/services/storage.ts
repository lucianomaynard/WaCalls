import { apiGet, apiDelete } from "@/lib/api";
import { getClientId } from "@/lib/client-id";
import type { StorageStatus, StorageForm } from "@/types/storage";

const jsonHeaders = () => ({ "X-Client-Id": getClientId(), "Content-Type": "application/json" });

export const getStorage = () => apiGet<StorageStatus>("/api/storage");

export const saveStorage = async (form: StorageForm): Promise<StorageStatus> => {
  const r = await fetch("/api/storage", { method: "PUT", headers: jsonHeaders(), body: JSON.stringify(form) });
  const data = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error((data as { error?: string }).error || `Erro ${r.status}`);
  return data as StorageStatus;
};

export const deleteStorage = () => apiDelete("/api/storage");

export const testStorage = async (form: StorageForm): Promise<{ ok: boolean; bucket_exists: boolean }> => {
  const r = await fetch("/api/storage/test", { method: "POST", headers: jsonHeaders(), body: JSON.stringify(form) });
  const data = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error((data as { error?: string }).error || `Erro ${r.status}`);
  return data as { ok: boolean; bucket_exists: boolean };
};
