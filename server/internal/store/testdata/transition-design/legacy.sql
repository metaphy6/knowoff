-- Synthetic but relationally realistic legacy head-8 data. Values are fake;
-- image/text history, consent, references and value are retained verbatim.
INSERT INTO accounts(id,nickname,locale) VALUES
('00000000-0000-0000-0000-000000000001','fixture-author','tr'),
('00000000-0000-0000-0000-000000000002','fixture-player','en');
INSERT INTO profiles(account_id,overall_points,non_converted_points,xp,contributor_credits) VALUES
('00000000-0000-0000-0000-000000000001',500,300,40,ARRAY['legacy-paid-pack:author']),
('00000000-0000-0000-0000-000000000002',120,120,10,ARRAY['legacy-community:winner']);
INSERT INTO noin_wallets(account_id,balance) VALUES('00000000-0000-0000-0000-000000000001',125);
INSERT INTO noin_ledger(account_id,event_type,amount,reason,payload,server_day) VALUES
('00000000-0000-0000-0000-000000000001','purchase',200,'legacy purchase','{"transaction_id":"fixture-verified"}','2025-01-02'),
('00000000-0000-0000-0000-000000000001','spend',-75,'legacy theme pack','{"pack_tag":"legacy-paid-pack"}','2025-01-02');
INSERT INTO daily_noin_earned(account_id,server_day,earned) VALUES('00000000-0000-0000-0000-000000000001','2025-01-02',12);
INSERT INTO daily_quickplay_counts(account_id,server_day,count) VALUES('00000000-0000-0000-0000-000000000001','2025-01-02',2);
INSERT INTO entitlements(account_id,entitlement_type,value,active_until) VALUES
('00000000-0000-0000-0000-000000000001','theme_pack','legacy-paid-pack',NULL),
('00000000-0000-0000-0000-000000000001','premium_yearly','fixture-premium','2028-02-01T00:00:00Z');
INSERT INTO store_purchases(id,account_id,platform,product_id,transaction_id,amount,verified_at,refunded_at,raw_receipt) VALUES
('30000000-0000-0000-0000-000000000001','00000000-0000-0000-0000-000000000001','google_play','fixture-noin','fixture-verified',200,'2025-01-02T10:00:00Z',NULL,'{"fixture":"verified receipt","provider_token":"not-a-real-token"}'),
('30000000-0000-0000-0000-000000000002','00000000-0000-0000-0000-000000000001','app_store','fixture-noin','fixture-refunded',50,'2025-01-01T10:00:00Z','2025-01-02T10:00:00Z','{"fixture":"refunded receipt","transaction_id":"fixture-refunded"}');
INSERT INTO portal_terms(version,title,body,active_from) VALUES('fixture-legacy-terms','Fixture contribution terms','Fixture retained commercial use and modification consent','2024-01-01T00:00:00Z');
INSERT INTO portal_submissions(id,account_id,media_type,content,asset_ref,asset_blob,status,tags,nown_id,pack_tag,terms_version,terms_accepted_at) VALUES
('10000000-0000-0000-0000-000000000001','00000000-0000-0000-0000-000000000001','image',NULL,'fixture-static-image',decode('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aCfoAAAAASUVORK5CYII=','base64'),'published',ARRAY['legacy','nown'],NULL,'legacy-paid-pack','fixture-legacy-terms','2025-01-01T00:00:00Z'),
('10000000-0000-0000-0000-000000000002','00000000-0000-0000-0000-000000000001','text','Spare key',NULL,NULL,'published',ARRAY['legacy','response'],'10000000-0000-0000-0000-000000000001','legacy-paid-pack','fixture-legacy-terms','2025-01-01T00:00:00Z'),
('10000000-0000-0000-0000-000000000003','00000000-0000-0000-0000-000000000001','text','Çay molası',NULL,NULL,'approved',ARRAY['legacy','item'],NULL,NULL,'fixture-legacy-terms','2025-01-01T00:00:00Z'),
('10000000-0000-0000-0000-000000000004','00000000-0000-0000-0000-000000000002','image',NULL,'fixture-rejected-image',decode('89504e470d0a1a0a','hex'),'rejected',ARRAY['legacy'],NULL,NULL,'fixture-legacy-terms','2025-01-01T00:00:00Z');
INSERT INTO challenge_topics(id,week_start,week_end,nown_media_id,winner_entry_id) VALUES
('20000000-0000-0000-0000-000000000001','2025-01-06','2025-01-12','10000000-0000-0000-0000-000000000001','10000000-0000-0000-0000-000000000002');
-- The entry UUID deliberately equals a submission UUID: source namespaces matter.
INSERT INTO challenge_entries(id,account_id,topic_id,entry_type,content,asset_ref,asset_blob,terms_version,terms_accepted_at,status,vote_count,slot_number) VALUES
('10000000-0000-0000-0000-000000000002','00000000-0000-0000-0000-000000000002','20000000-0000-0000-0000-000000000001','image',NULL,'fixture-challenge-image',decode('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aCfoAAAAASUVORK5CYII=','base64'),'fixture-legacy-terms','2025-01-06T00:00:00Z','approved',1,1),
('20000000-0000-0000-0000-000000000002','00000000-0000-0000-0000-000000000001','20000000-0000-0000-0000-000000000001','text','An extra chair',NULL,NULL,'fixture-legacy-terms','2025-01-06T00:00:00Z','approved',0,2);
INSERT INTO challenge_votes(account_id,topic_id,entry_id) VALUES('00000000-0000-0000-0000-000000000001','20000000-0000-0000-0000-000000000001','10000000-0000-0000-0000-000000000002');
INSERT INTO challenge_winners(topic_id,entry_id,account_id,noin_payout_granted_at) VALUES('20000000-0000-0000-0000-000000000001','10000000-0000-0000-0000-000000000002','00000000-0000-0000-0000-000000000002','2025-01-13T00:00:00Z');
INSERT INTO reports(report_type,reporter_id,target_media_id,reason) VALUES('media','00000000-0000-0000-0000-000000000002','10000000-0000-0000-0000-000000000001','fixture review');
INSERT INTO feedback(account_id,type,message,context_snapshot) VALUES('00000000-0000-0000-0000-000000000001','idea','fixture legacy feedback','{"pack_tag":"legacy-paid-pack","submission_id":"10000000-0000-0000-0000-000000000001"}');
