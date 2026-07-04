// Estado da integração Chatwoot de uma sessão (o token NUNCA volta do backend).
export type ChatwootStatus = {
  connected: boolean;
  webhook: string;
  has_token: boolean;
  base_url?: string;
  account_id?: string;
  inbox_id?: string;
  inbox_identifier?: string;
  updated_at?: string;
};

// Payload enviado ao salvar. access_token vazio = manter o atual.
export type ChatwootForm = {
  base_url: string;
  account_id: string;
  inbox_id: string;
  access_token: string;
  inbox_identifier: string;
};
