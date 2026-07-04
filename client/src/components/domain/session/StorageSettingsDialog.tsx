import { useEffect, useState } from "react";
import { Loader2, Save, Trash2, PlugZap } from "lucide-react";
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
import { getStorage, saveStorage, deleteStorage, testStorage } from "@/services/storage";
import type { StorageForm } from "@/types/storage";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

const empty: StorageForm = {
  provider: "minio",
  endpoint: "",
  bucket: "",
  region: "",
  access_key: "",
  secret_key: "",
  prefix: "gravacoes",
  use_ssl: true,
  enabled: true,
};

export const StorageSettingsDialog = ({ open, onOpenChange }: Props) => {
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [removing, setRemoving] = useState(false);
  const [configured, setConfigured] = useState(false);
  const [hasSecret, setHasSecret] = useState(false);
  const [form, setForm] = useState<StorageForm>(empty);

  const set = <K extends keyof StorageForm>(k: K, v: StorageForm[K]) => setForm((f) => ({ ...f, [k]: v }));

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    setLoading(true);
    getStorage()
      .then((s) => {
        if (cancelled) return;
        setConfigured(s.configured);
        setHasSecret(s.has_secret);
        setForm({
          provider: s.provider ?? "minio",
          endpoint: s.endpoint ?? "",
          bucket: s.bucket ?? "",
          region: s.region ?? "",
          access_key: s.access_key ?? "",
          secret_key: "", // nunca preenchemos o secret (vazio = manter atual)
          prefix: s.prefix ?? "gravacoes",
          use_ssl: s.use_ssl ?? true,
          enabled: s.enabled ?? true,
        });
      })
      .catch((e) => toast.error((e as Error).message))
      .finally(() => !cancelled && setLoading(false));
    return () => {
      cancelled = true;
    };
  }, [open]);

  const validate = (): string | null => {
    if (!form.endpoint.trim()) return "Endpoint é obrigatório (ex.: minio.exemplo.com).";
    if (!form.bucket.trim()) return "Bucket é obrigatório.";
    if (!form.access_key.trim()) return "Access Key é obrigatório.";
    if (!hasSecret && !form.secret_key.trim()) return "Secret Key é obrigatório.";
    return null;
  };

  const handleTest = async () => {
    const err = validate();
    if (err) return toast.error(err);
    setTesting(true);
    try {
      const r = await testStorage(form);
      if (r.bucket_exists) toast.success("Conexão OK — bucket encontrado.");
      else toast.warning("Credenciais OK, mas o bucket não existe.");
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setTesting(false);
    }
  };

  const handleSave = async () => {
    const err = validate();
    if (err) return toast.error(err);
    setSaving(true);
    try {
      const s = await saveStorage(form);
      setConfigured(s.configured);
      setHasSecret(s.has_secret);
      set("secret_key", "");
      toast.success("Armazenamento salvo.");
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const handleRemove = async () => {
    if (!window.confirm("Remover a configuração de armazenamento? As novas gravações voltam a ficar só no disco local.")) return;
    setRemoving(true);
    try {
      await deleteStorage();
      setConfigured(false);
      setHasSecret(false);
      setForm(empty);
      toast.success("Configuração removida.");
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setRemoving(false);
    }
  };

  const busy = saving || removing || testing;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Armazenamento de Gravações</DialogTitle>
          <DialogDescription>
            Onde salvar as gravações das chamadas. Compatível com MinIO, Amazon S3 e qualquer storage S3.
            Sem config, as gravações ficam só no disco local.
          </DialogDescription>
        </DialogHeader>

        {loading ? (
          <div className="flex items-center justify-center py-10 text-muted-foreground">
            <Loader2 className="mr-2 h-5 w-5 animate-spin" /> Carregando…
          </div>
        ) : (
          <div className="space-y-4 py-2">
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label htmlFor="st-provider">Provedor</Label>
                <Input id="st-provider" placeholder="minio" value={form.provider} onChange={(e) => set("provider", e.target.value)} />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="st-region">Region</Label>
                <Input id="st-region" placeholder="us-east-1" value={form.region} onChange={(e) => set("region", e.target.value)} />
              </div>
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="st-endpoint">Endpoint</Label>
              <Input id="st-endpoint" placeholder="minio.servemei.com.br" value={form.endpoint} onChange={(e) => set("endpoint", e.target.value)} />
              <p className="text-xs text-muted-foreground">Host:porta sem http/https. Ex.: minio.exemplo.com ou s3.amazonaws.com</p>
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label htmlFor="st-bucket">Bucket</Label>
                <Input id="st-bucket" placeholder="gravacoes" value={form.bucket} onChange={(e) => set("bucket", e.target.value)} />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="st-prefix">Prefixo/pasta</Label>
                <Input id="st-prefix" placeholder="gravacoes" value={form.prefix} onChange={(e) => set("prefix", e.target.value)} />
              </div>
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="st-access">Access Key</Label>
              <Input id="st-access" value={form.access_key} onChange={(e) => set("access_key", e.target.value)} />
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="st-secret">Secret Key</Label>
              <Input
                id="st-secret"
                type="password"
                placeholder={hasSecret ? "•••••• (deixe vazio p/ manter)" : "secret key"}
                value={form.secret_key}
                onChange={(e) => set("secret_key", e.target.value)}
              />
            </div>

            <div className="flex flex-col gap-2 pt-1">
              <label className="flex items-center gap-2 text-sm">
                <input type="checkbox" checked={form.use_ssl} onChange={(e) => set("use_ssl", e.target.checked)} />
                Usar HTTPS (SSL) na conexão
              </label>
              <label className="flex items-center gap-2 text-sm">
                <input type="checkbox" checked={form.enabled} onChange={(e) => set("enabled", e.target.checked)} />
                Enviar as gravações para este storage
              </label>
            </div>
          </div>
        )}

        <DialogFooter className="gap-2 sm:justify-between">
          <div className="flex gap-2">
            <Button variant="destructive" size="sm" onClick={handleRemove} disabled={busy || loading || !configured}>
              {removing ? <Loader2 className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}
              Remover
            </Button>
            <Button variant="outline" size="sm" onClick={handleTest} disabled={busy || loading}>
              {testing ? <Loader2 className="h-4 w-4 animate-spin" /> : <PlugZap className="h-4 w-4" />}
              Testar
            </Button>
          </div>
          <Button className="bg-green-600 text-white hover:bg-green-700" onClick={handleSave} disabled={busy || loading}>
            {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
            Salvar
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};
