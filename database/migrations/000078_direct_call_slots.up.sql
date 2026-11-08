-- direct_call_slots: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE direct_call_slots (
    user_id uuid NOT NULL,
    call_id uuid NOT NULL
);

-- migrate:constraints

ALTER TABLE ONLY direct_call_slots
    ADD CONSTRAINT direct_call_slots_pkey PRIMARY KEY (user_id);

CREATE INDEX direct_call_slots_call ON direct_call_slots USING btree (call_id);

-- migrate:references

ALTER TABLE ONLY direct_call_slots
    ADD CONSTRAINT direct_call_slots_call_id_fkey FOREIGN KEY (call_id) REFERENCES direct_calls(id) ON DELETE RESTRICT;

ALTER TABLE ONLY direct_call_slots
    ADD CONSTRAINT direct_call_slots_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT;
