ALTER TABLE public.order_notifications DROP CONSTRAINT order_notifications_state_check;
ALTER TABLE public.order_notifications ADD CHECK(state IN ('queued','sending','sent','failed','unknown','validated'));
ALTER TABLE public.order_notifications
 ADD COLUMN submission_key uuid UNIQUE,
 ADD COLUMN payload jsonb CHECK(jsonb_typeof(payload)='object' AND octet_length(payload::text)<=524288),
 ADD COLUMN started_at timestamptz,
 ADD COLUMN finished_at timestamptz,
 ADD COLUMN message_id text CHECK(octet_length(message_id) BETWEEN 1 AND 512);
-- Existing unobserved submissions cannot safely be resent.
UPDATE public.order_notifications SET state='unknown',finished_at=clock_timestamp() WHERE state='sending';
CREATE INDEX ON public.order_notifications(created_at,attempt_id,kind) WHERE state='queued';
CREATE FUNCTION public.protect_notification_delivery() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF ROW(NEW.attempt_id,NEW.kind,NEW.created_at) IS DISTINCT FROM ROW(OLD.attempt_id,OLD.kind,OLD.created_at)
 OR (OLD.submission_key IS NOT NULL AND ROW(NEW.submission_key,NEW.payload,NEW.started_at) IS DISTINCT FROM ROW(OLD.submission_key,OLD.payload,OLD.started_at))
 OR (OLD.state IN ('sent','failed','unknown','validated') AND NEW IS DISTINCT FROM OLD)
 OR (NEW.state<>OLD.state AND NOT ((OLD.state='queued' AND NEW.state='sending' AND NEW.submission_key IS NOT NULL AND NEW.payload IS NOT NULL AND NEW.started_at IS NOT NULL)
  OR (OLD.state='sending' AND NEW.state IN ('sent','failed','unknown','validated') AND NEW.finished_at IS NOT NULL))) THEN
 RAISE EXCEPTION 'notification submission is immutable';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER notification_delivery_immutable BEFORE UPDATE ON public.order_notifications FOR EACH ROW EXECUTE FUNCTION public.protect_notification_delivery();
