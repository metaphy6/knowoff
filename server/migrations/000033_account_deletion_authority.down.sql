LOCK TABLE public.privacy_deletion_capabilities,public.privacy_deletion_intents IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM public.privacy_deletion_capabilities) OR EXISTS(SELECT 1 FROM public.privacy_deletion_intents) THEN RAISE EXCEPTION 'refusing rollback with retained deletion authority'; END IF;
END $$;
DROP FUNCTION public.privacy_expire_deletion_intents(integer);
DROP FUNCTION public.privacy_deletion_intent_status(uuid,bytea);
DROP FUNCTION public.privacy_security_revoke_deletion(uuid);
DROP FUNCTION public.privacy_confirm_deletion(uuid,bytea,uuid,bytea,uuid,bytea,uuid);
DROP FUNCTION public.privacy_complete_deletion_oauth(uuid,text,text);
DROP FUNCTION public.privacy_claim_deletion_oauth(bytea,text);
DROP FUNCTION public.privacy_begin_deletion_oauth(uuid,bytea,timestamptz,text,bytea,text,text,uuid);
DROP FUNCTION public.privacy_begin_deletion_intent(uuid,uuid,bytea,bytea,timestamptz);
DROP FUNCTION public.privacy_enroll_capability(uuid,bytea,uuid,bytea);
DROP FUNCTION public.privacy_begin_enrollment(uuid,uuid,bytea,timestamptz,bigint,text,timestamptz,text);
DROP TABLE public.privacy_deletion_intents;
DROP TABLE public.privacy_deletion_capabilities;
