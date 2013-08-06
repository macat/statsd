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

SET default_with_oids = true;

--
-- Name: groups; Type: TABLE; Schema: public; Owner: kozo; Tablespace: 
--

CREATE TABLE groups (
    id uuid,
    name text,
    created timestamp with time zone
);


ALTER TABLE public.groups OWNER TO kozo;

--
-- PostgreSQL database dump complete
--

