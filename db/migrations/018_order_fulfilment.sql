CREATE TABLE public.order_fulfilment (
 attempt_id text PRIMARY KEY REFERENCES public.payment_attempts(id),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 state text NOT NULL DEFAULT 'awaiting_dispatch' CHECK(state IN ('awaiting_dispatch','dispatched')),
 carrier text NOT NULL DEFAULT '' CHECK(octet_length(carrier)<=320),
 tracking_reference text NOT NULL DEFAULT '' CHECK(octet_length(tracking_reference)<=480),
 tracking_url text NOT NULL DEFAULT '' CHECK(octet_length(tracking_url)<=2048),
 dispatched_at timestamptz,
 CHECK((state='awaiting_dispatch' AND dispatched_at IS NULL) OR (state='dispatched' AND dispatched_at IS NOT NULL))
);
CREATE FUNCTION public.protect_order_fulfilment() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM public.payment_attempts WHERE id=NEW.attempt_id AND state='paid') THEN
  RAISE EXCEPTION 'fulfilment requires confirmed payment';
 END IF;
 IF TG_OP='UPDATE' AND (NEW.attempt_id<>OLD.attempt_id OR NEW.revision<>OLD.revision+1 OR (OLD.state='dispatched' AND (NEW.state<>'dispatched' OR NEW.dispatched_at IS DISTINCT FROM OLD.dispatched_at))) THEN
  RAISE EXCEPTION 'invalid fulfilment transition';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER order_fulfilment_guard BEFORE INSERT OR UPDATE ON public.order_fulfilment FOR EACH ROW EXECUTE FUNCTION public.protect_order_fulfilment();
INSERT INTO public.order_fulfilment(attempt_id) SELECT id FROM public.payment_attempts WHERE state='paid';
