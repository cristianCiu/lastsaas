import { useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { Download, Mail } from 'lucide-react';
import Alert from '../../components/ui/Alert';
import Button from '../../components/ui/Button';
import ConfirmModal from '../../components/ConfirmModal';
import Input from '../../components/ui/Input';
import { purchasingApi } from './api';
import type { PurchaseOrder } from './types';
import { getErrorMessage } from '../../utils/errors';

export default function DocumentActions({ order, orderId, canRead, canSend }: { order: PurchaseOrder; orderId: string; canRead: boolean; canSend: boolean }) {
  const [email, setEmail] = useState(''); const [confirm, setConfirm] = useState(false); const [message, setMessage] = useState('');
  const download = useMutation({ mutationFn: () => purchasingApi.document(orderId), onSuccess: blob => { const url = URL.createObjectURL(blob); const a = document.createElement('a'); a.href = url; a.download = `${order.orderNumber}.pdf`; a.click(); URL.revokeObjectURL(url); }, onError: e => setMessage(getErrorMessage(e)) });
  const send = useMutation({ mutationFn: () => purchasingApi.sendDocument(orderId, { recipientEmail: email.trim().toLowerCase(), idempotencyKey: crypto.randomUUID() }), onSuccess: () => { setConfirm(false); setMessage(`Purchase order sent to ${email.trim().toLowerCase()}.`); setEmail(''); }, onError: e => { setConfirm(false); setMessage(getErrorMessage(e)); } });
  const sendable = ['approved', 'ordered', 'partially_received', 'received'].includes(order.status);
  return <section className="rounded-2xl border border-dark-800 bg-dark-900/60 p-5"><div className="flex items-start gap-3"><Mail className="mt-0.5 h-5 w-5 text-primary-400" /><div><h2 className="font-semibold text-white">Order document</h2><p className="mt-1 text-sm text-dark-400">Download a branded PDF anytime. Email delivery is always explicit and available after approval.</p></div></div>{message && <Alert className="mt-4" variant={send.isSuccess ? 'success' : 'error'}>{message}</Alert>}<div className="mt-5 flex flex-col gap-3 sm:flex-row sm:items-end"><Button variant="secondary" onClick={() => download.mutate()} disabled={!canRead || download.isPending} className="inline-flex items-center justify-center gap-2"><Download className="h-4 w-4" />{download.isPending ? 'Preparing…' : 'Download PDF'}</Button>{canSend && sendable && <><Input label="Recipient email" type="email" required value={email} onChange={e => setEmail(e.target.value)} placeholder="supplier@example.com" /><Button onClick={() => setConfirm(true)} disabled={!email.trim() || send.isPending} className="inline-flex items-center justify-center gap-2"><Mail className="h-4 w-4" />{send.isPending ? 'Sending…' : 'Send explicitly'}</Button></>}</div>{canSend && !sendable && <p className="mt-3 text-xs text-dark-500">The document can be emailed only after the order is approved.</p>}<ConfirmModal open={confirm} onClose={() => setConfirm(false)} onConfirm={() => send.mutate()} title="Send purchase-order document" message={`Send the branded PDF for ${order.orderNumber} to ${email.trim().toLowerCase()}? This email is explicit and cannot be recalled.`} confirmLabel="Send document" loading={send.isPending} /></section>;
}
