-- Canonical demo / e2e dataset.
--
-- The server applies this file on every boot while E2E_TEST_MODE=1 (see
-- applySeed in seed.go): all tables are wiped first, then the dataset below is
-- inserted. Playwright e2e and the docker demo compose therefore always start
-- from the exact same state, and every container spin-up re-seeds it.
--
-- 40 users: one admin, alice (the focal demo account) connected to six other
-- users, three further connected users, and 30 "mockNN" placeholder accounts
-- that exercise list pagination (20 per page). Specific test users (neww:,
-- dave, eve) are deliberately left out: the e2e specs provision them at
-- runtime and rely on them not existing yet.

-- Wipe all data. Foreign keys would cascade, but deleting rows explicitly
-- keeps the wipe deterministic and independent of cascade order.
DELETE FROM claims;
DELETE FROM connections;
DELETE FROM items;
DELETE FROM lists;
DELETE FROM users;
DELETE FROM metadata_cache;

-- ---------------------------------------------------------------------------
-- Users
-- ---------------------------------------------------------------------------
INSERT INTO users (id, auth_user_id, username, display_name, is_admin, birthday, holiday_reset, allow_suggestions, theme_style, share_code) VALUES
('e7af1e25-b63e-4695-8e32-8b679f366c69',    'e2e:admin',   'admin',   'Ada Root',           1, '',      '',       1, 'aurora', ''),
('aa9c9b0a-b54e-4771-9fba-25cc8da989a7',    'e2e:alice',   'alice',   'Alice Apple',        0, '05-15', '',       1, 'aurora', ''),
('36fd0773-c639-4534-9333-712d223aacc0',      'e2e:bob',     'bob',     'Bob Banana',         0, '06-01', '',       1, 'aurora', ''),
('104f7b10-f992-46ed-9d1e-33dcad415508',    'e2e:carol',   'carol',   'Carol Cherry',       0, '03-20', '',       1, 'aurora', ''),
('dd5e180a-c9cd-4d88-9b0d-428e7ce99565',     'e2e:dana',    'dana',    'Dana Date',          0, '02-14', 'mothers', 1, 'aurora', ''),
('6786c003-4c1e-4b01-9557-9ec37596fcb0',     'e2e:eric',    'eric',    'Eric Elderberry',    0, '08-08', '',       1, 'aurora', ''),
('2ee6ef4d-6524-4bf4-a3d0-5071cdea7aa8',     'e2e:faye',    'faye',    'Faye Fig',           0, '11-11', '',       1, 'aurora', ''),
('5cee61e7-b2b8-41b8-a669-bc6b2f71c2a0',   'e2e:george',  'george',  'George Gooseberry',  0, '01-15', '',       1, 'aurora', ''),
('bcccfa3a-62a9-45e4-8da4-238e5c5615fe',    'e2e:helen',   'helen',   'Helen Honeydew',     0, '04-04', '',       1, 'aurora', ''),
('46d1af21-176f-4794-883e-3d87a2d8b922',     'e2e:ivan',    'ivan',    'Ivan Kiwi',          0, '09-09', '',       1, 'aurora', '');

INSERT INTO users (id, auth_user_id, username, display_name, is_admin, birthday, holiday_reset, allow_suggestions, theme_style, share_code) VALUES
('629e91cb-87af-4fd2-a942-43802eaa49da', 'e2e:mock01', 'mock01', 'Mock User 01', 0, '', '', 1, 'aurora', ''),
('c77b1f10-93ec-4194-ab31-007ce82bcdd1', 'e2e:mock02', 'mock02', 'Mock User 02', 0, '', '', 1, 'aurora', ''),
('1cf8891c-11da-4393-ba7d-4bfdc3fdf468', 'e2e:mock03', 'mock03', 'Mock User 03', 0, '', '', 1, 'aurora', ''),
('af9c831a-996f-4444-933f-2f351ca93f73', 'e2e:mock04', 'mock04', 'Mock User 04', 0, '', '', 1, 'aurora', ''),
('603ab298-58af-4caa-a77e-14d52465e2eb', 'e2e:mock05', 'mock05', 'Mock User 05', 0, '', '', 1, 'aurora', ''),
('12595112-d151-4dda-88be-64b846e0407d', 'e2e:mock06', 'mock06', 'Mock User 06', 0, '', '', 1, 'aurora', ''),
('0288790c-21ad-4a96-adb9-eab468fa439a', 'e2e:mock07', 'mock07', 'Mock User 07', 0, '', '', 1, 'aurora', ''),
('bed2850f-893a-4fa5-8ba3-6e45b34cd9df', 'e2e:mock08', 'mock08', 'Mock User 08', 0, '', '', 1, 'aurora', ''),
('dd4bfe62-8d7a-4b6a-8f90-817d30cb4746', 'e2e:mock09', 'mock09', 'Mock User 09', 0, '', '', 1, 'aurora', ''),
('1df4e5ff-f26b-481d-94f0-717d3f783bad', 'e2e:mock10', 'mock10', 'Mock User 10', 0, '', '', 1, 'aurora', ''),
('07921885-4c46-4c2a-af8e-f47c78309acd', 'e2e:mock11', 'mock11', 'Mock User 11', 0, '', '', 1, 'aurora', ''),
('ad3aa8cf-d3d5-438f-8967-5b75382d06b5', 'e2e:mock12', 'mock12', 'Mock User 12', 0, '', '', 1, 'aurora', ''),
('e9a74999-3229-473a-a5d3-b7c9e4e92d7b', 'e2e:mock13', 'mock13', 'Mock User 13', 0, '', '', 1, 'aurora', ''),
('5a2c6772-7984-4d71-8ba9-74527286db90', 'e2e:mock14', 'mock14', 'Mock User 14', 0, '', '', 1, 'aurora', ''),
('9403d8b1-8028-4758-90b2-d772d742e9ab', 'e2e:mock15', 'mock15', 'Mock User 15', 0, '', '', 1, 'aurora', ''),
('6fe0f13b-eb71-449c-8169-7c66c011297f', 'e2e:mock16', 'mock16', 'Mock User 16', 0, '', '', 1, 'aurora', ''),
('e52a4217-caff-4f73-9e52-10b3a5768385', 'e2e:mock17', 'mock17', 'Mock User 17', 0, '', '', 1, 'aurora', ''),
('6845c948-a6e0-47c2-ad02-71d7c6de155a', 'e2e:mock18', 'mock18', 'Mock User 18', 0, '', '', 1, 'aurora', ''),
('34bbec8f-8bf3-4c95-a14c-e701ed8c4b06', 'e2e:mock19', 'mock19', 'Mock User 19', 0, '', '', 1, 'aurora', ''),
('860bfc4b-c360-4f7d-a8c9-3b5c98830f9d', 'e2e:mock20', 'mock20', 'Mock User 20', 0, '', '', 1, 'aurora', ''),
('a461b19c-ed12-4663-bae1-f95964ac729e', 'e2e:mock21', 'mock21', 'Mock User 21', 0, '', '', 1, 'aurora', ''),
('ca9172d9-e6fe-473e-860f-3f322879ab1f', 'e2e:mock22', 'mock22', 'Mock User 22', 0, '', '', 1, 'aurora', ''),
('81ed93d8-b58d-4d26-8357-b77440750231', 'e2e:mock23', 'mock23', 'Mock User 23', 0, '', '', 1, 'aurora', ''),
('3323b3c8-82d7-4fd9-9287-2b49006e96f0', 'e2e:mock24', 'mock24', 'Mock User 24', 0, '', '', 1, 'aurora', ''),
('f89ec4f2-643d-41a2-b0d8-047d19036434', 'e2e:mock25', 'mock25', 'Mock User 25', 0, '', '', 1, 'aurora', ''),
('94092366-6cf3-47cf-b2e2-0d62aeeeb506', 'e2e:mock26', 'mock26', 'Mock User 26', 0, '', '', 1, 'aurora', ''),
('1bcfc75f-bc24-4da2-8509-77ded2998634', 'e2e:mock27', 'mock27', 'Mock User 27', 0, '', '', 1, 'aurora', ''),
('f8f9185d-0b3a-4cb3-9579-f2b7fc9e7563', 'e2e:mock28', 'mock28', 'Mock User 28', 0, '', '', 1, 'aurora', ''),
('d9ab233a-09be-4317-a6c5-4a07561441c7', 'e2e:mock29', 'mock29', 'Mock User 29', 0, '', '', 1, 'aurora', ''),
('5ba45dd6-ea10-4775-9d45-8ee30c49d977', 'e2e:mock30', 'mock30', 'Mock User 30', 0, '', '', 1, 'aurora', '');

-- ---------------------------------------------------------------------------
-- Lists: exactly one active (non-archived) list per user. Ids are UUIDs (as in
-- production), so demo list URLs never leak usernames; no archived lists are
-- seeded so the archive flow starts from a clean slate.
-- ---------------------------------------------------------------------------
INSERT INTO lists (id, user_id, label) VALUES
('f9d55fab-ea0c-4f5a-aa5c-fbacc86658cd',  'e7af1e25-b63e-4695-8e32-8b679f366c69',   'Active'),
('2b066115-e0c9-45c9-91e9-d9ac1081a796',  'aa9c9b0a-b54e-4771-9fba-25cc8da989a7',   'Active'),
('350026a8-da87-414a-953d-df4b4e9659ed',    '36fd0773-c639-4534-9333-712d223aacc0',     'Active'),
('3d69638c-75b2-44f9-a10c-b33f4b9b9ae5',  '104f7b10-f992-46ed-9d1e-33dcad415508',   'Active'),
('4d37483c-6895-4a9b-895c-0092c296cf23',   'dd5e180a-c9cd-4d88-9b0d-428e7ce99565',    'Active'),
('d50da6b6-3b63-4e4a-8b2f-3e409c5941ae',   '6786c003-4c1e-4b01-9557-9ec37596fcb0',    'Active'),
('c34c4bdc-94c2-4063-9a12-ca61f2b0709a',   '2ee6ef4d-6524-4bf4-a3d0-5071cdea7aa8',    'Active'),
('2bc47c05-44fe-4ebc-a0f5-04c62fef263e', '5cee61e7-b2b8-41b8-a669-bc6b2f71c2a0',  'Active'),
('85cbc38c-d0a8-4e78-aea7-6d8c2c3d7e48',  'bcccfa3a-62a9-45e4-8da4-238e5c5615fe',   'Active'),
('db4192bd-91a8-4b10-95bb-87bce4f534ad',   '46d1af21-176f-4794-883e-3d87a2d8b922',    'Active');

INSERT INTO lists (id, user_id, label) VALUES
('d8541ebd-74de-497e-a3d6-dcf8b12b4470', '629e91cb-87af-4fd2-a942-43802eaa49da', 'Active'),
('4be7d3df-cc9d-40ce-a222-48ddc7a296d5', 'c77b1f10-93ec-4194-ab31-007ce82bcdd1', 'Active'),
('387feec0-45a0-4e2b-8779-d51a0d88cca6', '1cf8891c-11da-4393-ba7d-4bfdc3fdf468', 'Active'),
('da856d3d-0673-4f2e-a8ea-468bbc5c0171', 'af9c831a-996f-4444-933f-2f351ca93f73', 'Active'),
('1a22c712-d85d-4073-a062-6d5cae78130c', '603ab298-58af-4caa-a77e-14d52465e2eb', 'Active'),
('4305bf87-163c-4a10-907d-33e41daea37d', '12595112-d151-4dda-88be-64b846e0407d', 'Active'),
('07ecfa39-84fe-4fea-b01a-dfc5fddfcb5d', '0288790c-21ad-4a96-adb9-eab468fa439a', 'Active'),
('061038fc-e698-423f-b27d-9dd1817bb578', 'bed2850f-893a-4fa5-8ba3-6e45b34cd9df', 'Active'),
('e2d16755-7c05-4f98-9d5e-dfc46dea874e', 'dd4bfe62-8d7a-4b6a-8f90-817d30cb4746', 'Active'),
('ef2a990d-78d2-410c-8bc2-cc1fa18ae83a', '1df4e5ff-f26b-481d-94f0-717d3f783bad', 'Active'),
('5b906084-6549-49ce-ae26-a7a4f882d8c0', '07921885-4c46-4c2a-af8e-f47c78309acd', 'Active'),
('bd7f39ce-168d-4646-bd04-b07976babde8', 'ad3aa8cf-d3d5-438f-8967-5b75382d06b5', 'Active'),
('c87084d6-d404-455c-b498-88933528040d', 'e9a74999-3229-473a-a5d3-b7c9e4e92d7b', 'Active'),
('12645400-302c-4a15-b64b-42ff22bafb80', '5a2c6772-7984-4d71-8ba9-74527286db90', 'Active'),
('2d48ce32-735f-4de1-b4f0-60da1c9e0a0f', '9403d8b1-8028-4758-90b2-d772d742e9ab', 'Active'),
('b7ecf347-e2a9-44ae-8aa0-7312180d333e', '6fe0f13b-eb71-449c-8169-7c66c011297f', 'Active'),
('04e884e1-6322-4f4a-ab02-9d12d86e328c', 'e52a4217-caff-4f73-9e52-10b3a5768385', 'Active'),
('fe18150a-cb35-451d-8ddd-9042b962b966', '6845c948-a6e0-47c2-ad02-71d7c6de155a', 'Active'),
('229bbea1-1aa5-4da6-97be-c8a05324c557', '34bbec8f-8bf3-4c95-a14c-e701ed8c4b06', 'Active'),
('4a56bef6-acac-4ca1-add8-232f7d18b21a', '860bfc4b-c360-4f7d-a8c9-3b5c98830f9d', 'Active'),
('bd254b14-674c-4ce5-ad93-3f00839ba01a', 'a461b19c-ed12-4663-bae1-f95964ac729e', 'Active'),
('680ae06d-276b-4f66-bdaf-d9ce5efba989', 'ca9172d9-e6fe-473e-860f-3f322879ab1f', 'Active'),
('892772ff-4cd8-4b3d-bb5e-6d6fb93418a2', '81ed93d8-b58d-4d26-8357-b77440750231', 'Active'),
('82e0eb52-347e-40aa-94b5-c633f91bf3e6', '3323b3c8-82d7-4fd9-9287-2b49006e96f0', 'Active'),
('fbf11603-bb9e-4ecb-9198-32b7de424815', 'f89ec4f2-643d-41a2-b0d8-047d19036434', 'Active'),
('c219ea45-0edc-484e-82c0-709618afab3b', '94092366-6cf3-47cf-b2e2-0d62aeeeb506', 'Active'),
('a7f3c2b5-e7af-4a22-a69b-6bce2cced4d1', '1bcfc75f-bc24-4da2-8509-77ded2998634', 'Active'),
('8eedb21f-9ca2-4401-a3a1-ede3dad463ad', 'f8f9185d-0b3a-4cb3-9579-f2b7fc9e7563', 'Active'),
('72eed871-e959-49ff-a0f5-93ef1dceb5c0', 'd9ab233a-09be-4317-a6c5-4a07561441c7', 'Active'),
('dcea33e1-f05b-4ec8-b510-202e23eb78ca', '5ba45dd6-ea10-4775-9d45-8ee30c49d977', 'Active');

-- ---------------------------------------------------------------------------
-- Connections (bidirectional pairs). alice is connected to bob, carol, dana,
-- eric, faye and george; helen and ivan are deliberately NOT connected to her
-- so demo users can experience the "no access" limitation.
-- ---------------------------------------------------------------------------
INSERT INTO connections (user_id, other_user_id) VALUES
('aa9c9b0a-b54e-4771-9fba-25cc8da989a7',  '36fd0773-c639-4534-9333-712d223aacc0'),
('36fd0773-c639-4534-9333-712d223aacc0',    'aa9c9b0a-b54e-4771-9fba-25cc8da989a7'),
('aa9c9b0a-b54e-4771-9fba-25cc8da989a7',  '104f7b10-f992-46ed-9d1e-33dcad415508'),
('104f7b10-f992-46ed-9d1e-33dcad415508',  'aa9c9b0a-b54e-4771-9fba-25cc8da989a7'),
('aa9c9b0a-b54e-4771-9fba-25cc8da989a7',  'dd5e180a-c9cd-4d88-9b0d-428e7ce99565'),
('dd5e180a-c9cd-4d88-9b0d-428e7ce99565',   'aa9c9b0a-b54e-4771-9fba-25cc8da989a7'),
('aa9c9b0a-b54e-4771-9fba-25cc8da989a7',  '6786c003-4c1e-4b01-9557-9ec37596fcb0'),
('6786c003-4c1e-4b01-9557-9ec37596fcb0',   'aa9c9b0a-b54e-4771-9fba-25cc8da989a7'),
('aa9c9b0a-b54e-4771-9fba-25cc8da989a7',  '2ee6ef4d-6524-4bf4-a3d0-5071cdea7aa8'),
('2ee6ef4d-6524-4bf4-a3d0-5071cdea7aa8',   'aa9c9b0a-b54e-4771-9fba-25cc8da989a7'),
('aa9c9b0a-b54e-4771-9fba-25cc8da989a7',  '5cee61e7-b2b8-41b8-a669-bc6b2f71c2a0'),
('5cee61e7-b2b8-41b8-a669-bc6b2f71c2a0', 'aa9c9b0a-b54e-4771-9fba-25cc8da989a7'),
('36fd0773-c639-4534-9333-712d223aacc0',    '104f7b10-f992-46ed-9d1e-33dcad415508'),
('104f7b10-f992-46ed-9d1e-33dcad415508',  '36fd0773-c639-4534-9333-712d223aacc0'),
('36fd0773-c639-4534-9333-712d223aacc0',    'dd5e180a-c9cd-4d88-9b0d-428e7ce99565'),
('dd5e180a-c9cd-4d88-9b0d-428e7ce99565',   '36fd0773-c639-4534-9333-712d223aacc0'),
('104f7b10-f992-46ed-9d1e-33dcad415508',  'dd5e180a-c9cd-4d88-9b0d-428e7ce99565'),
('dd5e180a-c9cd-4d88-9b0d-428e7ce99565',   '104f7b10-f992-46ed-9d1e-33dcad415508'),
('dd5e180a-c9cd-4d88-9b0d-428e7ce99565',   '6786c003-4c1e-4b01-9557-9ec37596fcb0'),
('6786c003-4c1e-4b01-9557-9ec37596fcb0',   'dd5e180a-c9cd-4d88-9b0d-428e7ce99565'),
('2ee6ef4d-6524-4bf4-a3d0-5071cdea7aa8',   '5cee61e7-b2b8-41b8-a669-bc6b2f71c2a0'),
('5cee61e7-b2b8-41b8-a669-bc6b2f71c2a0', '2ee6ef4d-6524-4bf4-a3d0-5071cdea7aa8'),
('2ee6ef4d-6524-4bf4-a3d0-5071cdea7aa8',   'bcccfa3a-62a9-45e4-8da4-238e5c5615fe'),
('bcccfa3a-62a9-45e4-8da4-238e5c5615fe',  '2ee6ef4d-6524-4bf4-a3d0-5071cdea7aa8'),
('46d1af21-176f-4794-883e-3d87a2d8b922',   '5cee61e7-b2b8-41b8-a669-bc6b2f71c2a0'),
('5cee61e7-b2b8-41b8-a669-bc6b2f71c2a0', '46d1af21-176f-4794-883e-3d87a2d8b922');

-- ---------------------------------------------------------------------------
-- Items. Suggested ideas are tagged with their author; private ideas are
-- owner-only. Claims are placed by connected users on other people's lists.
-- ---------------------------------------------------------------------------
INSERT INTO items (id, list_id, title, url, notes, sort_order, visibility, suggested_by, suggested_by_id) VALUES
('i-alice-01', '2b066115-e0c9-45c9-91e9-d9ac1081a796', 'Lé Creuset kettle',          'https://example.com/kettle-1',        '',                      0, 'public',    '',            ''),
('i-alice-02', '2b066115-e0c9-45c9-91e9-d9ac1081a796', 'Espresso machine',           'https://example.com/espresso-machine', 'with grinder',          1, 'public',    '',            ''),
('i-alice-03', '2b066115-e0c9-45c9-91e9-d9ac1081a796', 'Nordic throw blanket',       '',                                   '',                      2, 'public',    '',            ''),
('i-alice-04', '2b066115-e0c9-45c9-91e9-d9ac1081a796', 'Bike helmet',                '',                                   'city commuter style',   3, 'public',    '',            ''),
('i-alice-05', '2b066115-e0c9-45c9-91e9-d9ac1081a796', 'Terracotta planter set',     '',                                   'from the garden show',  4, 'suggested', 'Carol Cherry', '104f7b10-f992-46ed-9d1e-33dcad415508'),
('i-alice-06', '2b066115-e0c9-45c9-91e9-d9ac1081a796', 'Weekend spa vouchers',       '',                                   'keep under wraps',      5, 'private',   '',            '');

INSERT INTO items (id, list_id, title, url, notes, sort_order, visibility, suggested_by, suggested_by_id) VALUES
('i-dana-01', '4d37483c-6895-4a9b-895c-0092c296cf23', 'Stand mixer',         'https://example.com/stand-mixer', 'the 5-quart model',     0, 'public',    '',             ''),
('i-dana-02', '4d37483c-6895-4a9b-895c-0092c296cf23', 'Yoga mat',            '',                                '',                      1, 'public',    '',             ''),
('i-dana-03', '4d37483c-6895-4a9b-895c-0092c296cf23', 'Watercolor set',      '',                                'for the holiday cards', 2, 'public',    '',             ''),
('i-dana-04', '4d37483c-6895-4a9b-895c-0092c296cf23', 'Bird-watching book',  '',                                '',                      3, 'suggested', 'Eric Elderberry', '6786c003-4c1e-4b01-9557-9ec37596fcb0');

INSERT INTO items (id, list_id, title, url, notes, sort_order, visibility, suggested_by, suggested_by_id) VALUES
('i-bob-01', '350026a8-da87-414a-953d-df4b4e9659ed', 'Noise-cancelling headphones', 'https://example.com/headphones', '',              0, 'public',    '',            ''),
('i-bob-02', '350026a8-da87-414a-953d-df4b4e9659ed', 'Running shoes',               '',                                'size 10.5',     1, 'public',    '',            ''),
('i-bob-03', '350026a8-da87-414a-953d-df4b4e9659ed', 'Trail map stash',             '',                                '',              2, 'suggested', 'Carol Cherry', '104f7b10-f992-46ed-9d1e-33dcad415508');

INSERT INTO items (id, list_id, title, url, notes, sort_order, visibility, suggested_by, suggested_by_id) VALUES
('i-carol-01', '3d69638c-75b2-44f9-a10c-b33f4b9b9ae5', 'Book club pick',        '',                      '',              0, 'public',    '',          ''),
('i-carol-02', '3d69638c-75b2-44f9-a10c-b33f4b9b9ae5', 'Farmers market tote',   '',                      '',              1, 'public',    '',          ''),
('i-carol-03', '3d69638c-75b2-44f9-a10c-b33f4b9b9ae5', 'Dinner date night',     '',                      'the tasting menu', 2, 'suggested', 'Bob Banana', '36fd0773-c639-4534-9333-712d223aacc0');

INSERT INTO items (id, list_id, title, url, notes, sort_order, visibility, suggested_by, suggested_by_id) VALUES
('i-eric-01', 'd50da6b6-3b63-4e4a-8b2f-3e409c5941ae', 'Camping lantern',   'https://example.com/lantern', '', 0, 'public', '', ''),
('i-eric-02', 'd50da6b6-3b63-4e4a-8b2f-3e409c5941ae', 'Hex key set',       '',                            '', 1, 'public', '', '');

INSERT INTO items (id, list_id, title, url, notes, sort_order, visibility, suggested_by, suggested_by_id) VALUES
('i-faye-01', 'c34c4bdc-94c2-4063-9a12-ca61f2b0709a', 'Sourdough starter kit', 'https://example.com/sourdough',   '',   0, 'public',    '',              ''),
('i-faye-02', 'c34c4bdc-94c2-4063-9a12-ca61f2b0709a', 'Herb garden stand',     '',                                 '',   1, 'public',    '',              ''),
('i-faye-03', 'c34c4bdc-94c2-4063-9a12-ca61f2b0709a', 'Botany course',         '',                                 '',   2, 'suggested', 'George Gooseberry', '5cee61e7-b2b8-41b8-a669-bc6b2f71c2a0');

INSERT INTO items (id, list_id, title, url, notes, sort_order, visibility, suggested_by, suggested_by_id) VALUES
('i-george-01', '2bc47c05-44fe-4ebc-a0f5-04c62fef263e', 'Bird feeder camera',    'https://example.com/birdfeeder-cam', '', 0, 'public', '', ''),
('i-george-02', '2bc47c05-44fe-4ebc-a0f5-04c62fef263e', 'Outdoor thermometer',   '',                                  '', 1, 'public', '', '');

INSERT INTO items (id, list_id, title, url, notes, sort_order, visibility, suggested_by, suggested_by_id) VALUES
('i-helen-01', '85cbc38c-d0a8-4e78-aea7-6d8c2c3d7e48', 'Potted orchid',     '',                       '',               0, 'public',    '',          ''),
('i-helen-02', '85cbc38c-d0a8-4e78-aea7-6d8c2c3d7e48', 'Silk pillowcases',  '',                       '',               1, 'public',    '',          ''),
('i-helen-03', '85cbc38c-d0a8-4e78-aea7-6d8c2c3d7e48', 'Gift wrapping set', '',                       'always runs out', 2, 'suggested', 'Faye Fig',  '2ee6ef4d-6524-4bf4-a3d0-5071cdea7aa8');

INSERT INTO items (id, list_id, title, url, notes, sort_order, visibility, suggested_by, suggested_by_id) VALUES
('i-ivan-01', 'db4192bd-91a8-4b10-95bb-87bce4f534ad', 'Label maker',       '',                     'for the pantry', 0, 'public', '', ''),
('i-ivan-02', 'db4192bd-91a8-4b10-95bb-87bce4f534ad', 'Espresso tamper',   '',                     '',               1, 'public', '', '');

-- ---------------------------------------------------------------------------
-- Claims. carol claims one of alice's items and one of bob's; eric claims one
-- of dana's; bob claims one of carol's. Bob has NO claim on alice's active
-- list (the claim e2e badge expects "1 claimed" for bob's own claim).
-- ---------------------------------------------------------------------------
INSERT INTO claims (item_id, user_id) VALUES
('i-alice-02', '104f7b10-f992-46ed-9d1e-33dcad415508'),
('i-dana-03',  '6786c003-4c1e-4b01-9557-9ec37596fcb0'),
('i-bob-02',   '104f7b10-f992-46ed-9d1e-33dcad415508'),
('i-carol-02', '36fd0773-c639-4534-9333-712d223aacc0');

-- ---------------------------------------------------------------------------
-- Link preview metadata for the seeded item URLs, so tiles render without any
-- network access (the /api/metadata handler serves these from the cache).
-- ---------------------------------------------------------------------------
INSERT INTO metadata_cache (url, title, description, image) VALUES
('https://example.com/kettle-1',        'Le Creuset Kettle 1.6L',        'Iconic cast-iron kettle.',   ''),
('https://example.com/espresso-machine','Espresso machine with grinder', 'Dual boiler espresso maker.', ''),
('https://example.com/stand-mixer',     'Stand mixer (5-quart)',         'Legendary stand mixer.',     ''),
('https://example.com/headphones',      'Noise-cancelling headphones',   'Wireless, with ANC.',        ''),
('https://example.com/lantern',         'Rechargeable camping lantern',  'Runs all weekend.',         ''),
('https://example.com/sourdough',       'Sourdough Starter Kit',         'Includes a pre-ferment jar.', ''),
('https://example.com/birdfeeder-cam',  'Bird Feeder Camera',            'Capture backyard birds.',    '');
