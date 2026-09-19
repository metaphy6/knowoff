LOCK TABLE public.billing_provider_work,public.billing_verification_slots IN ACCESS EXCLUSIVE MODE;
DO $retained$ BEGIN
 IF EXISTS(SELECT 1 FROM public.privacy_step_receipts WHERE step='billing') OR EXISTS(SELECT 1 FROM public.billing_provider_work) OR EXISTS(SELECT 1 FROM public.billing_verification_slots) THEN RAISE EXCEPTION 'billing provider work retained'; END IF;
END $retained$;
DROP FUNCTION public.privacy_begin_billing_drain(uuid),public.privacy_claim_billing_attempt(uuid,uuid,text,uuid,integer),public.privacy_finish_billing_attempt(uuid,uuid,text,bigint,uuid,bytea,bytea,text),public.privacy_finish_billing_drain(uuid);
ALTER TABLE public.privacy_step_receipts DROP CONSTRAINT privacy_step_receipt_kind;
ALTER TABLE public.privacy_step_receipts ADD CONSTRAINT privacy_step_receipt_kind CHECK((step='profile' AND result_code='profile_removed') OR (step='credentials' AND result_code='credentials_removed') OR (step='installations' AND result_code='installations_removed') OR (step='installation_evidence' AND result_code='installation_evidence_purged'));
DROP TABLE public.billing_provider_work,public.billing_verification_slots;
DROP FUNCTION public.billing_verification_slot_guard(),public.billing_provider_work_guard();
ALTER TABLE public.billing_subscription_sources DROP CONSTRAINT billing_subscription_initial_purchase_unique;
