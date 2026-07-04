import { apiGet, apiDelete } from "@/lib/api";
import { getClientId } from "@/lib/client-id";
import type { ChatwootStatus, ChatwootForm } from "@/types/chatwoot";

export const getChatwoot = (sid: string) =>
  apiGet<ChatwootStatus>(`/api/sessions/${sid}/chatwoot`);

// PUT com erro "limpo": extrai o campo {error} do backend quando falha.
export const saveChatwoot = async (sid: string, form: ChatwootForm): Promise<ChatwootStatus> => {
  const r = await fetch(`/api/sessions/${sid}/chatwoot`, {
    method: "PUT",
    headers: { "X-Client-Id": getClientId(), "Content-Type": "application/json" },
    body: JSON.stringify(form),
  });
  const data = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error((data as { error?: string }).error || `Erro ${r.status}`);
  return data as ChatwootStatus;
};

export const disconnectChatwoot = (sid: string) => apiDelete(`/api/sessions/${sid}/chatwoot`);
