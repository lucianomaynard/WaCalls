import { useEffect, useState } from "react";
import { Copy, Loader2, Save, Unplug } from "lucide-react";
import { toast } from "sonner";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { getChatwoot, saveChatwoot, disconnectChatwoot } from "@/services/chatwoot";

type Props = {
  sid: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

const isValidUrl = (v: string) => {
  try {
    const u = new URL(v);
    return u.protocol === "http:" || u.protocol === "https:";
  } catch {
    return false;
  }
};

export const ChatwootIntegrationDialog = ({ sid, open, onOpenChange }: Props) => {
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [disconnecting, setDisconnecting] = useState(false);
  const [connected, setConnected] = useState(false);

  const [baseUrl, setBaseUrl] = useState("");
  const [accountId, setAccountId] = useState("");
  const [inboxId, setInboxId] = useState("");
  const [accessToken, setAccessToken] = useState("");
  const [inboxIdentifier, setInboxIdentifier] = useState("");
  const [webhook, setWebhook] = useState("");

  // Carrega a config atual sempre que abrir.
  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    setLoading(true);
    getChatwoot(sid)
      .then((s) => {
        if (cancelled) return;
        setConnected(s.connected);
        setBaseUrl(s.base_url ?? "");
        setAccountId(s.account_id ?? "");
        setInboxId(s.inbox_id ?? "");
        setInboxIdentifier(s.inbox_identifier ?? "");
        setWebhook(s.webhook ?? "");
        setAccessToken(""); // nunca preenchemos o token (fica vazio = manter atual)
      })
      .catch((e) => toast.error((e as Error).message))
      .finally(() => !cancelled && setLoading(false));
    return () => {
      cancelled = true;
    };
  }, [open, sid]);

  const validate = (): string | null => {
    if (!isValidUrl(baseUrl)) return "URL do Chatwoot inválida (use http/https).";
    if (!accountId.trim()) return "Account ID é obrigatório.";
    if (!/^\d+$/.test(inboxId.trim())) return "Inbox ID deve ser um número.";
    return null;
  };

  const handleSave = async () => {
    const err = validate();
    if (err) {
      toast.error(err);
      return;
    }
    setSaving(true);
    try {
      const s = await saveChatwoot(sid, {
        base_url: baseUrl.trim(),
        account_id: accountId.trim(),
        inbox_id: inboxId.trim(),
        access_token: accessToken, // vazio = mantém o atual
        inbox_identifier: inboxIdentifier.trim(),
      });
      setConnected(s.connected);
      setWebhook(s.webhook ?? webhook);
      setAccessToken("");
      toast.success("Integração com Chatwoot salva.");
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const handleDisconnect = async () => {
    if (!window.confirm("Desconectar a integração com o Chatwoot desta sessão?")) return;
    setDisconnecting(true);
    try {
      await disconnectChatwoot(sid);
      setConnected(false);
      setBaseUrl("");
      setAccountId("");
      setInboxId("");
      setInboxIdentifier("");
      setAccessToken("");
      toast.success("Integração desconectada.");
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setDisconnecting(false);
    }
  };

  const copyWebhook = async () => {
    try {
      await navigator.clipboard.writeText(webhook);
      toast.success("Webhook copiado.");
    } catch {
      toast.error("Não foi possível copiar.");
    }
  };

  const busy = saving || disconnecting;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Integração com Chatwoot</DialogTitle>
          <DialogDescription>
            Conecte esta sessão do WaCalls a uma Inbox do tipo API do Chatwoot.
          </DialogDescription>
        </DialogHeader>

        {loading ? (
          <div className="flex items-center justify-center py-10 text-muted-foreground">
            <Loader2 className="mr-2 h-5 w-5 animate-spin" /> Carregando…
          </div>
        ) : (
          <div className="space-y-4 py-2">
            <div className="space-y-1.5">
              <Label htmlFor="cw-url">URL do Chatwoot</Label>
              <Input
                id="cw-url"
                placeholder="https://sac.servemei.com.br/"
                value={baseUrl}
                onChange={(e) => setBaseUrl(e.target.value)}
              />
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label htmlFor="cw-account">Account ID</Label>
                <Input
                  id="cw-account"
                  placeholder="1"
                  value={accountId}
                  onChange={(e) => setAccountId(e.target.value)}
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="cw-inbox">Inbox ID</Label>
                <Input
                  id="cw-inbox"
                  inputMode="numeric"
                  placeholder="29"
                  value={inboxId}
                  onChange={(e) => setInboxId(e.target.value)}
                />
              </div>
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="cw-token">Access Token</Label>
              <Input
                id="cw-token"
                type="password"
                placeholder="deixe vazio p/ manter o atual"
                value={accessToken}
                onChange={(e) => setAccessToken(e.target.value)}
              />
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="cw-identifier">Inbox Identifier</Label>
              <Input
                id="cw-identifier"
                placeholder="uwGbujvqyfnn2YpR373rNyjh"
                value={inboxIdentifier}
                onChange={(e) => setInboxIdentifier(e.target.value)}
              />
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="cw-webhook">Webhook (cole na inbox do Chatwoot)</Label>
              <div className="flex gap-2">
                <Input id="cw-webhook" readOnly value={webhook} className="font-mono text-xs" />
                <Button type="button" variant="outline" size="icon" onClick={copyWebhook} disabled={!webhook}>
                  <Copy className="h-4 w-4" />
                </Button>
              </div>
              <p className="text-xs text-muted-foreground">
                Copie esta URL para a Inbox API dentro do Chatwoot.
              </p>
            </div>
          </div>
        )}

        <DialogFooter className="gap-2 sm:justify-between">
          <Button variant="destructive" onClick={handleDisconnect} disabled={busy || loading || !connected}>
            {disconnecting ? <Loader2 className="h-4 w-4 animate-spin" /> : <Unplug className="h-4 w-4" />}
            Desconectar
          </Button>
          <Button
            className="bg-green-600 text-white hover:bg-green-700"
            onClick={handleSave}
            disabled={busy || loading}
          >
            {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
            Salvar
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};
