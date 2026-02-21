-- Allow data_extracts without an attachment (inline CSV upload).
ALTER TABLE data_extracts DROP CONSTRAINT data_extracts_attachment_id_fkey;
ALTER TABLE data_extracts ALTER COLUMN attachment_id DROP NOT NULL;
ALTER TABLE data_extracts
    ADD CONSTRAINT data_extracts_attachment_id_fkey
    FOREIGN KEY (attachment_id) REFERENCES attachments(id) ON DELETE RESTRICT;
