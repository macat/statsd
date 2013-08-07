--
-- PostgreSQL database dump
--

SET statement_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SET check_function_bodies = false;
SET client_min_messages = warning;

SET search_path = public, pg_catalog;

SET default_tablespace = '';

SET default_with_oids = false;

--
-- Name: users; Type: TABLE; Schema: public; Owner: kozo; Tablespace: 
--

CREATE TABLE users (
    id uuid NOT NULL,
    name text,
    email text,
    created timestamp with time zone
);


ALTER TABLE public.users OWNER TO kozo;

--
-- Name: users_pkey; Type: CONSTRAINT; Schema: public; Owner: kozo; Tablespace: 
--

ALTER TABLE ONLY users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- PostgreSQL database dump complete
--

