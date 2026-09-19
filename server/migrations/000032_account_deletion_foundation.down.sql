LOCK TABLE public.privacy_requests,public.privacy_step_receipts,public.account_deletion_fences IN ACCESS EXCLUSIVE MODE;
DO $$
BEGIN
 IF EXISTS(SELECT 1 FROM public.privacy_requests) OR EXISTS(SELECT 1 FROM public.privacy_step_receipts) OR EXISTS(SELECT 1 FROM public.account_deletion_fences) THEN
  RAISE EXCEPTION 'refusing rollback with retained deletion requests';
 END IF;
END $$;
DROP FUNCTION public.account_deletion_status(bytea);
DROP FUNCTION public.privacy_erase_profile_batch(uuid,integer);
DROP FUNCTION public.privacy_bind_suppression(uuid,bigint,bytea);
DROP FUNCTION public.privacy_prepare_verified_request(uuid,uuid,bytea,bytea);
DROP TABLE public.account_deletion_fences;
DROP TABLE public.privacy_step_receipts;
DROP TABLE public.privacy_requests;
