ALTER TABLE public.shops
 ADD COLUMN tagline text NOT NULL DEFAULT '' CHECK (char_length(tagline)<=160),
 ADD COLUMN description text NOT NULL DEFAULT '' CHECK (char_length(description)<=2000),
 ADD COLUMN contact_email text NOT NULL DEFAULT '' CHECK (char_length(contact_email)<=254),
 ADD COLUMN revision bigint NOT NULL DEFAULT 1 CHECK (revision>0);
