ALTER TABLE public.payment_attempts ADD COLUMN reconcile_after timestamptz NOT NULL DEFAULT clock_timestamp()+interval '30 seconds';
CREATE INDEX payment_reconciliation_due ON public.payment_attempts(reconcile_after) WHERE state IN ('submitting','pending','unknown') AND provider_id IS NOT NULL;
